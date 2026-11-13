// SPDX-License-Identifier: AGPL-3.0-only

// The HTML the server renders a local post's text as (internal/app/content/text.go, ParsedText.HTML), for the samples
// of the catalog: a paragraph at each blank line, a line break at one, and its URLs, mentions and hashtags linked as
// Mastodon links them, a URL shown without its scheme and cut after 30 characters. Every mention resolves, to the
// profile page on the server of its handle.

const escapes: Readonly<Record<string, string>> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&#34;",
  "'": "&#39;",
};
const escape = (s: string) => s.replace(/[&<>"']/g, (c) => escapes[c] ?? c);

const rel = 'rel="nofollow noopener noreferrer" target="_blank"';

// URLs, mentions (@user or @user@host) and hashtags, in that order of precedence.
const token = /(https?:\/\/[^\s]+)|(@[\w.-]+(?:@[\w-]+(?:\.[\w-]+)+)?)|(#[\p{L}\p{N}_]+)/gu;

function urlDisplay(url: string): string {
  const scheme = url.indexOf("://") + 3;
  const prefix = url.slice(0, url.slice(scheme).startsWith("www.") ? scheme + 4 : scheme);
  const rest = url.slice(prefix.length);
  const hidden = `<span class="invisible">${escape(prefix)}</span>`;
  if (rest.length <= 30) return `${hidden}<span>${escape(rest)}</span>`;
  return `${hidden}<span class="ellipsis">${escape(rest.slice(0, 30))}</span><span class="invisible">${escape(rest.slice(30))}</span>`;
}

function linked(line: string, host: string): string {
  let out = "";
  let at = 0;
  for (const m of line.matchAll(token)) {
    const [whole, url, mention, tag] = m;
    out += escape(line.slice(at, m.index));
    if (url) out += `<a href="${escape(url)}" ${rel}>${urlDisplay(url)}</a>`;
    else if (mention) {
      const [user = "", domain = host] = mention.slice(1).split("@");
      out += `<span class="h-card"><a href="https://${domain}/@${escape(user)}" class="u-url mention" ${rel}>@<span>${escape(user)}</span></a></span>`;
    } else if (tag) {
      const name = tag.slice(1);
      out += `<a href="https://${host}/tags/${encodeURIComponent(name.toLowerCase())}" class="mention hashtag" ${rel}>#<span>${escape(name)}</span></a>`;
    }
    at = m.index + whole.length;
  }
  return out + escape(line.slice(at));
}

/** The HTML of a post's text, as a status's `content`, on the server `host`. */
export function serverHtml(text: string, host = "my.example"): string {
  return text
    .split(/\n{2,}/)
    .map(
      (para) =>
        `<p>${para
          .split("\n")
          .map((line) => linked(line, host))
          .join("<br>")}</p>`,
    )
    .join("");
}
