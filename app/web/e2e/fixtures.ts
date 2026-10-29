// SPDX-License-Identifier: AGPL-3.0-only
// Shared E2E fixtures and assertions (ADR-0027). Specs import `test` and `expect` from here, not from
// @playwright/test, so every spec gets the same axe configuration.
import AxeBuilder from "@axe-core/playwright";
import { test as base, expect, type Locator, type Page } from "@playwright/test";
import { previewOrigin, serverURL } from "./server-origin.ts";

/** The axe rule tags for WCAG 2.2 AA (ADR-0027, web-client/README.md Y1). */
const axeTags = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] as const;

type Fixtures = {
  /** Returns an AxeBuilder for the page, limited to `axeTags`; chain `.include()` / `.exclude()` to scope it. */
  makeAxeBuilder: () => AxeBuilder;
  /**
   * Serves the HTML documents of `vite preview` (the client) with the project's locale in
   * <html lang dir>, as `my serve` writes the instance's default locale into its shell (ADR-0028);
   * `vite preview` serves index.html as it is. The runtime config's `apiOrigin` becomes the preview's own
   * origin, which the run picks (server-origin.ts), so that the client's API and OAuth requests stay on it,
   * where specs can answer them with `page.route`. The documents carry the Content-Security-Policy of the
   * server's shell (`shellPolicy`), and the images of `servedImage` are answered on the preview's origin.
   * Pages of a real `my serve` are left alone.
   */
  shellLocale: string;
};

/** The Content-Security-Policy of `shellPolicy`, fetched once per worker. */
let shellPolicyCache: Promise<string> | undefined;

/**
 * The Content-Security-Policy that `my serve` sends with the app's shell (ADR-0019 §9; e2e/server.ts), which
 * `vite preview` does not send: the pages of the client run under the same Trusted Types and sources as in
 * production, so a sink that the policy refuses (a string to DOMParser or innerHTML) fails the specs, as it
 * fails the app. The server is the one source of the policy, so nothing here copies it.
 */
function shellPolicy(): Promise<string> {
  shellPolicyCache ??= fetch(new URL("/home", serverURL)).then((response) => {
    const policy = response.headers.get("content-security-policy");
    if (policy === null) throw new Error(`${serverURL}/home sent no Content-Security-Policy`);
    return policy;
  });
  return shellPolicyCache;
}

/** The path under which the `shellLocale` fixture serves the images of `servedImage`. */
const imagePath = "/e2e-image/";

/**
 * A URL on the preview's own origin that serves the image of the `data:` URL `dataURL`, for the images that
 * specs make up. The shell's CSP takes images only from the app's own origin and `blob:` (`img-src 'self'
 * blob:`), as the server serves its media, so a `data:` URL in an API response would not load.
 */
export function servedImage(dataURL: string): string {
  return `${previewOrigin}${imagePath}${Buffer.from(dataURL).toString("base64url")}`;
}

/** The response of a URL of `servedImage`; null for any other URL. */
function imageOf(url: URL): { contentType: string; body: Buffer } | null {
  if (!url.pathname.startsWith(imagePath)) return null;
  const dataURL = Buffer.from(url.pathname.slice(imagePath.length), "base64url").toString();
  const match = /^data:([^;,]+)(;base64)?,(.*)$/s.exec(dataURL);
  if (match === null) throw new Error(`not a data: URL: ${dataURL}`);
  const [, contentType = "", base64, data = ""] = match;
  return {
    contentType,
    body: base64 === undefined ? Buffer.from(decodeURIComponent(data)) : Buffer.from(data, "base64"),
  };
}

/** Pseudo-locale and right-to-left languages the projects use; the rest are left to right. */
const rtlLocales: ReadonlySet<string> = new Set(["ar-XB"]);

/** The text direction of a project locale. */
/** `text` as a literal in a regular expression: a message's pseudo-locale (en-XA) text is wrapped in brackets. */
export function escapeRegExp(text: string): string {
  return text.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);
}

export const localeDir = (locale: string): "ltr" | "rtl" => (rtlLocales.has(locale) ? "rtl" : "ltr");

/**
 * Waits until no finite animation or transition of the document runs. A control that changes its variant
 * (Follow on the accent becoming Requested on a raised surface) fades its colours over a transition
 * (`motion-safe:transition-colors`), and axe's `color-contrast` reads the computed colours of the moment:
 * halfway through, the text and its background are both between their two values and can fail the contrast.
 * The page is checked as it rests, so the scan waits for the end values; animations that never end (a
 * spinner, a skeleton's pulse) are left running.
 */
async function settleAnimations(page: Page): Promise<void> {
  await page.evaluate(async () => {
    for (;;) {
      // getAnimations() brings the styles up to date first, so a transition that a class change starts is listed.
      const running = document
        .getAnimations()
        .filter(
          (animation) =>
            animation.playState === "running" && Number.isFinite(animation.effect?.getComputedTiming().endTime),
        );
      if (running.length === 0) return;
      // An animation cancelled on the way (its element removed) rejects `finished`; it no longer runs either.
      await Promise.allSettled(running.map((animation) => animation.finished));
    }
  });
}

/** An AxeBuilder whose scan starts once the page's animations and transitions have ended (`settleAnimations`). */
class SettledAxeBuilder extends AxeBuilder {
  readonly #settledPage: Page;

  constructor(page: Page) {
    super({ page });
    this.#settledPage = page;
  }

  override async analyze(): ReturnType<AxeBuilder["analyze"]> {
    await settleAnimations(this.#settledPage);
    return super.analyze();
  }
}

/**
 * An AxeBuilder for `page`, limited to `axeTags`, without React Aria's live announcer, that scans the page
 * once its animations and transitions have ended (`settleAnimations`). The announcer is no content of the
 * page: React Aria's Button announces the start and the end of its pending state there with a node that is
 * named by the button (aria-labelledby) and stays for 7 seconds, so a button that a navigation removed in
 * the meantime leaves a nameless node behind until then.
 */
export function axeBuilder(page: Page): AxeBuilder {
  return new SettledAxeBuilder(page).withTags([...axeTags]).exclude("[data-live-announcer]");
}

export const test = base.extend<Fixtures>({
  makeAxeBuilder: async ({ page }, use) => {
    await use(() => axeBuilder(page));
  },
  shellLocale: [
    async ({ page, locale }, use) => {
      const lang = locale ?? "en";
      // A real server (E2E_BASE_URL, or the `my serve` of the server-rendered pages) writes lang and dir itself.
      if (process.env["E2E_BASE_URL"] !== undefined) {
        await use(lang);
        return;
      }
      await page.route(
        (url) => url.origin === previewOrigin,
        async (route) => {
          const image = imageOf(new URL(route.request().url()));
          if (image !== null) {
            await route.fulfill(image);
            return;
          }
          if (route.request().resourceType() !== "document") {
            await route.fallback();
            return;
          }
          // The fetch reuses an idle keep-alive connection, which `vite preview` closes after 5 seconds; a
          // request sent as the server closes it is reset before any response. A browser sends such a GET
          // again on a new connection (RFC 9112 §9.3.1), so the fetch does too, once.
          const response = await route.fetch({ maxRetries: 1 });
          // The build's index.html links its entry relative to itself (base "./"), which resolves under
          // the path of a nested page (/tags/cats). `my serve` never sends that file: it writes the shell's
          // tags from the build manifest as root paths (e2e/deep-links.spec.ts checks it), so only here
          // are the links made root-relative.
          const body = (await response.text())
            .replace(/<html [^>]*>/, `<html lang="${lang}" dir="${localeDir(lang)}">`)
            .replace(/"apiOrigin":"[^"]*"/, `"apiOrigin":"${previewOrigin}"`)
            .replaceAll(/(src|href)="\.\//g, '$1="/');
          const headers = { ...response.headers(), "content-security-policy": await shellPolicy() };
          await route.fulfill({ response, body, headers });
        },
      );
      await use(lang);
    },
    { auto: true },
  ],
});

export { expect };

/** One axe violation reduced to what identifies it: the rule and the elements it fails on. */
type ViolationFingerprint = { rule: string; targets: string[] };

/**
 * Scans the page with axe and asserts that it has no violations. The assertion compares fingerprints
 * (rule ID and target selectors) rather than the full axe result, so a failure lists exactly which rule
 * failed on which elements and stays readable.
 */
export async function expectNoAxeViolations(makeAxeBuilder: () => AxeBuilder): Promise<void> {
  const results = await makeAxeBuilder().analyze();
  const fingerprints: ViolationFingerprint[] = results.violations.map((violation) => ({
    rule: violation.id,
    targets: violation.nodes.map((node) => node.target.join(" ")),
  }));
  expect(fingerprints, "axe violations (rule and target selectors)").toEqual([]);
}

/** Asserts that the page does not scroll horizontally at the current viewport width (web-client/README.md D3). */
export async function expectNoHorizontalScroll(page: Page): Promise<void> {
  const { scrollWidth, clientWidth, wide } = await page.evaluate(() => {
    const clientWidth = document.documentElement.clientWidth;
    // What lies past the edge outside a table's own scroller, to name the culprit in the failure.
    const wide = [...document.body.querySelectorAll("*")]
      .filter(
        (element) =>
          element.getBoundingClientRect().right > clientWidth + 1 &&
          element.closest('[role="group"][tabindex="0"]') === null,
      )
      .slice(0, 4)
      .map((element) => `${element.tagName.toLowerCase()} ${Math.round(element.getBoundingClientRect().width)}px`);
    return { scrollWidth: document.documentElement.scrollWidth, clientWidth, wide };
  });
  expect(
    scrollWidth,
    `scrollWidth ${scrollWidth}px exceeds clientWidth ${clientWidth}px (past the edge: ${wide.join(", ")})`,
  ).toBeLessThanOrEqual(clientWidth);
}

/**
 * Asserts that a tab is whole in its tab list, which scrolls sideways when the tabs do not fit: no part of
 * it is cut off by the list's edges or under the fade (as wide as the list's scroll-padding) of an edge with
 * more tabs behind it. Polls, since the list scrolls after a render.
 */
export async function expectTabWhollyVisible(tab: Locator): Promise<void> {
  await expect
    .poll(() =>
      tab.evaluate((element) => {
        const list = element.closest<HTMLElement>('[role="tablist"]');
        if (list === null) return "not in a tablist";
        const style = getComputedStyle(list);
        const rtl = style.direction === "rtl";
        const offset = Math.abs(list.scrollLeft);
        const start = offset > 1;
        const end = list.scrollWidth - list.clientWidth - offset > 1;
        const listRect = list.getBoundingClientRect();
        const tabRect = element.getBoundingClientRect();
        const left = listRect.left + ((rtl ? end : start) ? Number.parseFloat(style.scrollPaddingLeft) : 0);
        const right = listRect.right - ((rtl ? start : end) ? Number.parseFloat(style.scrollPaddingRight) : 0);
        // A pixel of slack for fractional positions.
        return tabRect.left >= left - 1 && tabRect.right <= right + 1
          ? "whole"
          : `${tabRect.left}–${tabRect.right} outside ${left}–${right} (scrollLeft ${list.scrollLeft})`;
      }),
    )
    .toBe("whole");
}

/**
 * Opens the page at `path` from the main navigation as a user of the project's width does (ADR-0039): with the
 * item of the tab bar or the rail that links to it, or below md with the link in the menu's sheet for the items
 * that are not in the tab bar; signed out, below lg, with the item behind the top bar's Menu button (a disclosure,
 * not a dialog). Call it once the current page has rendered.
 */
export async function openFromNavigation(page: Page, path: string, hasTouch: boolean): Promise<void> {
  const press = (target: Locator) => (hasTouch ? target.tap() : target.click());
  const nav = page.getByRole("navigation");
  // The rail's items stay in the document below md, hidden (display: none), and the other way round.
  const item = nav.locator(`a[href="${path}"]`).filter({ visible: true });
  if ((await item.count()) > 0) {
    await press(item);
    return;
  }
  await press(nav.getByRole("button"));
  await press(page.getByRole("dialog").or(nav).locator(`a[href="${path}"]`).filter({ visible: true }));
}

/**
 * Asserts that the header of every post card in `feed` (its author and time) takes at most two lines: the
 * display name and the time, and the handle under them on a narrow card (web-client/16-look-states-and-wording.md G7). A line is the
 * line height of the display name, the header's largest text; a pixel of slack covers fractional layout.
 */
export async function expectPostHeadersInTwoLines(feed: Locator): Promise<void> {
  const tall = await feed.getByRole("article").evaluateAll((articles) =>
    articles.flatMap((article) => {
      const header = article.querySelector("header");
      const name = header?.querySelector<HTMLElement>("[dir=auto]");
      if (header == null || name == null)
        return [`article ${article.getAttribute("aria-posinset") ?? "?"}: no header with a name`];
      const height = header.getBoundingClientRect().height;
      const lineHeight = Number.parseFloat(getComputedStyle(name).lineHeight);
      return height <= 2 * lineHeight + 1
        ? []
        : [`${name.textContent}: ${String(height)}px for a line height of ${String(lineHeight)}px`];
    }),
  );
  expect(tall, "post headers taller than two lines").toEqual([]);
}

/** The renditions of an image whose every size is served at `url` (api/openapi.yaml, MediaVariants). */
export function imageVariants(url: string): {
  small: { url: string };
  medium: { url: string };
  large: { url: string };
} {
  return { small: { url }, medium: { url }, large: { url } };
}
