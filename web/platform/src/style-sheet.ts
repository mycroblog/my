// SPDX-License-Identifier: AGPL-3.0-only

// A stylesheet the app lays over the whole page for a while (the draft of the instance theme, ADR-0059): constructed
// from its text and adopted by the document, after every stylesheet of the page, so that its rules win, and taken
// away again. A constructed stylesheet is no inline `<style>` and no `blob:` URL, which the page's policy
// (`style-src 'self'`) refuses; its text is the server's, built from checked token values. The stylesheets it stands
// in place of (the published theme's, which the shell links) are turned off meanwhile and on again after, so that
// what it leaves out is the default and not what they set.

/**
 * Lays the stylesheet of `text` over the page in place of the linked stylesheets whose path starts with `replacing`,
 * and returns what puts the page back as it was; nothing where the browser cannot construct a stylesheet.
 */
export function adoptStyleSheet(text: string, replacing?: string): () => void {
  if (typeof CSSStyleSheet === "undefined" || !("adoptedStyleSheets" in document)) return () => undefined;
  const sheet = new CSSStyleSheet();
  sheet.replaceSync(text);
  const replaced =
    replacing === undefined
      ? []
      : [...document.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]')].filter(
          (link) => !link.disabled && new URL(link.href, location.href).pathname.startsWith(replacing),
        );
  for (const link of replaced) link.disabled = true;
  document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
  return () => {
    document.adoptedStyleSheets = document.adoptedStyleSheets.filter((s) => s !== sheet);
    for (const link of replaced) link.disabled = false;
  };
}
