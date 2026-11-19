// SPDX-License-Identifier: AGPL-3.0-only

// A stylesheet the app lays over the whole page for a while (the draft of the instance theme, ADR-0059): constructed
// from its text and adopted by the document, after every stylesheet of the page, so that its rules win as the
// published theme's do, and taken away again. A constructed stylesheet is no inline `<style>` and no `blob:` URL,
// which the page's policy (`style-src 'self'`) refuses; its text is the server's, built from checked token values.

/**
 * Lays the stylesheet of `text` over the page, and returns what takes it away again; nothing where the browser cannot
 * construct a stylesheet.
 */
export function adoptStyleSheet(text: string): () => void {
  if (typeof CSSStyleSheet === "undefined" || !("adoptedStyleSheets" in document)) return () => undefined;
  const sheet = new CSSStyleSheet();
  sheet.replaceSync(text);
  document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
  return () => {
    document.adoptedStyleSheets = document.adoptedStyleSheets.filter((s) => s !== sheet);
  };
}
