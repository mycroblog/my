// SPDX-License-Identifier: AGPL-3.0-only
// The release notes page of every version of the site (ADR-0150 §12). The build carries only the page with an empty
// placeholder (src/content/docs/releases.mdx); the Worker reads the published releases from the GitHub REST API, keeps
// the answer in the Cache API of the data center, and writes the notes into the placeholder as the page is served. The
// browser only ever loads the page from the site, so the site's CSP needs no api.github.com, and the readers' views
// never reach GitHub's rate limit: each data center asks GitHub at most once per `fresh` and answers from the cache.

/** The repository whose GitHub Releases the page shows. */
const repository = "mycroblog/my";

/** The element of the page the notes go into. */
const placeholder = "#release-notes";

/** A published release, its notes already sanitized (sanitize). */
interface Release {
  tag: string;
  name: string;
  publishedAt: string;
  url: string;
  html: string;
}

/** The answer kept in the cache: the releases and when GitHub gave them. */
interface Cached {
  fetchedAt: number;
  releases: Release[];
}

/** How long a cached answer is served before the Worker asks GitHub again (in the background), in seconds. */
const fresh = 10 * 60;
/** How long the cache keeps an answer, served while GitHub cannot be reached, in seconds. */
const keep = 7 * 24 * 60 * 60;
/** The most pages of 100 releases read; each is a subrequest, of the 50 a request may make on the free plan. */
const maxPages = 10;

export interface ReleaseNotesDeps {
  /** fetch, to GitHub. */
  fetch: (request: Request) => Promise<Response>;
  cache: WorkerCache;
  ctx: WorkerContext;
  /** A GitHub token that can read the repository's public releases, or undefined to ask GitHub unauthenticated. */
  token: string | undefined;
  now: () => number;
}

/** Whether `pathname` is the release notes page of a version, in English or in a locale: /next/releases/, /v0.1/ja/releases/. */
export function isReleaseNotesPath(pathname: string): boolean {
  return /^\/[^/]+\/(?:[^/]+\/)?releases\/$/.test(pathname);
}

/**
 * The page `response` (an asset) with the release notes in its placeholder. With no answer from GitHub and none in the
 * cache, the page is served as built: its placeholder links GitHub Releases.
 */
export async function withReleaseNotes(response: Response, deps: ReleaseNotesDeps): Promise<Response> {
  if (response.status !== 200 || !(response.headers.get("content-type") ?? "").startsWith("text/html")) {
    return response;
  }
  const releases = await cachedReleases(deps);
  if (releases === undefined) {
    return response;
  }
  const html = renderReleases(releases);
  const page = new HTMLRewriter()
    .on(placeholder, {
      element(element) {
        element.setInnerContent(html, { html: true });
      },
    })
    .transform(response);
  const headers = new Headers(page.headers);
  // The page is no longer the asset its validators name, and it changes with every release.
  headers.delete("etag");
  headers.delete("last-modified");
  headers.set("cache-control", `public, max-age=${fresh}`);
  return new Response(page.body, { status: page.status, statusText: page.statusText, headers });
}

/** The key of the answer in the cache: a URL of the site that serves nothing. */
const cacheKey = new Request(`https://mycroblog.dev/_worker/releases/${repository}`);

/**
 * The releases from the cache, refreshed in the background once the answer is older than `fresh`; on a miss, from
 * GitHub. undefined when the cache has none and GitHub does not answer.
 */
async function cachedReleases(deps: ReleaseNotesDeps): Promise<Release[] | undefined> {
  const hit = await deps.cache.match(cacheKey);
  const cached = hit === undefined ? undefined : ((await hit.json()) as Cached);
  if (cached !== undefined) {
    if (deps.now() - cached.fetchedAt > fresh * 1000) {
      deps.ctx.waitUntil(refresh(deps).catch(() => undefined));
    }
    return cached.releases;
  }
  try {
    return await refresh(deps);
  } catch {
    return undefined;
  }
}

/** Reads the releases from GitHub and keeps them in the cache. A failure leaves the cache as it was. */
async function refresh(deps: ReleaseNotesDeps): Promise<Release[]> {
  const releases = await fetchReleases(deps);
  const body: Cached = { fetchedAt: deps.now(), releases };
  await deps.cache.put(
    cacheKey,
    new Response(JSON.stringify(body), {
      headers: { "content-type": "application/json", "cache-control": `max-age=${keep}` },
    }),
  );
  return releases;
}

interface GitHubRelease {
  tag_name: string;
  name: string | null;
  draft: boolean;
  prerelease: boolean;
  published_at: string | null;
  html_url: string;
  body_html?: string;
}

/**
 * The published stable releases of the repository, newest first, as GitHub lists them: the pages of the Link header,
 * the notes as HTML (the html media type gives body_html). Drafts and prereleases are left out, as the deploy does.
 */
async function fetchReleases(deps: Pick<ReleaseNotesDeps, "fetch" | "token">): Promise<Release[]> {
  const out: Release[] = [];
  let url: string | undefined = `https://api.github.com/repos/${repository}/releases?per_page=100`;
  for (let page = 0; url !== undefined && page < maxPages; page++) {
    const headers: Record<string, string> = {
      accept: "application/vnd.github.html+json",
      "user-agent": "mycroblog-docs-site",
      "x-github-api-version": "2022-11-28",
    };
    if (deps.token !== undefined && deps.token !== "") {
      headers["authorization"] = `Bearer ${deps.token}`;
    }
    const response = await deps.fetch(new Request(url, { headers }));
    if (!response.ok) {
      throw new Error(`GitHub answered ${response.status} for ${url}`);
    }
    for (const release of (await response.json()) as GitHubRelease[]) {
      if (release.draft || release.prerelease || release.published_at === null) {
        continue;
      }
      out.push({
        tag: release.tag_name,
        name: release.name || release.tag_name,
        publishedAt: release.published_at,
        url: release.html_url,
        html: await sanitize(release.body_html ?? ""),
      });
    }
    url = nextPage(response.headers.get("link"));
  }
  return out;
}

/** The URL of rel="next" in a Link header, if it is GitHub's API. */
function nextPage(link: string | null): string | undefined {
  const next = /<([^>]+)>;\s*rel="next"/.exec(link ?? "")?.[1];
  return next?.startsWith("https://api.github.com/") ? next : undefined;
}

/** The elements the notes keep; any other element is replaced by its content, or removed with it (dropped). */
const allowedElements = new Set(
  "a abbr b blockquote br code dd del details div dl dt em h1 h2 h3 h4 h5 h6 hr i img ins kbd li ol p pre q s samp small span strong sub summary sup table tbody td tfoot th thead tr tt ul".split(
    " ",
  ),
);
const droppedElements = new Set(
  "script style iframe object embed form input button textarea select template svg math".split(" "),
);
/** The attributes the notes keep; href and src only with an https URL or a #fragment. */
const allowedAttributes = new Set([
  "href",
  "src",
  "alt",
  "title",
  "id",
  "colspan",
  "rowspan",
  "align",
  "open",
  "datetime",
]);

/**
 * The notes of a release with only the elements and attributes of allowedElements and allowedAttributes. GitHub renders
 * body_html as it shows the release, but documents no guarantee about it, and the site serves it as its own HTML.
 */
export async function sanitize(html: string): Promise<string> {
  const rewritten = new HTMLRewriter()
    .on("*", {
      element(element) {
        const tag = element.tagName.toLowerCase();
        if (droppedElements.has(tag)) {
          element.remove();
          return;
        }
        if (!allowedElements.has(tag)) {
          element.removeAndKeepContent();
          return;
        }
        for (const [name, value] of [...element.attributes]) {
          const keep =
            allowedAttributes.has(name) &&
            (name !== "href" && name !== "src" ? true : /^(?:https:\/\/|#)/i.test(value.trim()));
          if (!keep) {
            element.removeAttribute(name);
          }
        }
      },
    })
    .transform(new Response(html, { headers: { "content-type": "text/html" } }));
  return rewritten.text();
}

const dateFormat = new Intl.DateTimeFormat("en", { dateStyle: "long", timeZone: "UTC" });

/** The HTML of the notes: one section per release, newest first, its heading linking the release on GitHub. */
function renderReleases(releases: Release[]): string {
  if (releases.length === 0) {
    return `<p>No release has been published yet. <a href="https://github.com/${repository}/releases">GitHub Releases</a> lists the releases as they come.</p>`;
  }
  return releases
    .map((release) => {
      const id = `release-${release.tag.replace(/[^A-Za-z0-9_-]/g, "-")}`;
      const url = release.url.startsWith("https://github.com/")
        ? release.url
        : `https://github.com/${repository}/releases`;
      const date = new Date(release.publishedAt);
      const time = Number.isNaN(date.getTime())
        ? ""
        : `<p><time datetime="${escape(release.publishedAt)}">${dateFormat.format(date)}</time></p>`;
      return `<section aria-labelledby="${id}"><h2 id="${id}"><a href="${escape(url)}">${escape(release.name)}</a></h2>${time}${release.html}</section>`;
    })
    .join("");
}

function escape(text: string): string {
  return text.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}
