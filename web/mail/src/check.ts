// SPDX-License-Identifier: AGPL-3.0-only

// The check of the email layout's HTML (ADR-0086 §2), which every mail client reads alike only when it keeps to:
// - tables: the layout is tables (table, tr, td) with text and links in their cells, no other block and no layout of
//   CSS (flex, grid, float, position);
// - inline styles: every look is an inline style; the one stylesheet, in the head, holds only the color schemes and
//   the dark colors, which a client that drops it does without;
// - no external resource but the logo: no font, stylesheet, script, frame or image is loaded, except the server's logo,
//   whose URL the server fills in; no script runs;
// - lang and dir: <html> has both, from the locale.
// `checkFrame` returns each problem as `<rule>: <what>`, none for a layout that keeps to them. frame.tsx refuses to
// write a layout with a problem, and `bun web/mail/src/check.ts <file>…` checks files.

/** The elements of the layout: the document, the tables and what their cells hold. */
const allowed = new Set([
  "html",
  "head",
  "meta",
  "title",
  "style",
  "body",
  "table",
  "tbody",
  "tr",
  "td",
  "a",
  "span",
  "strong",
  "br",
  "img",
]);

/** What loads a resource or runs a script, in any style, attribute or comment. */
const loads = /url\s*\(|@import|@font-face|expression\s*\(/i;
/** The CSS that lays out otherwise than a table. */
const layoutCss = /(?:^|;)\s*(?:display\s*:\s*(?:inline-)?(?:flex|grid)|float\s*:|position\s*:)/i;
/** A value of the template, which the server fills in. */
const action = /^\{\{[^{}]*\}\}$/;
/** The attributes that load what they name. */
const loading = new Set(["src", "srcset", "background", "poster", "data", "lowsrc", "dynsrc"]);

/** The stylesheet with the color schemes and the dark block taken out, which leaves nothing in a layout that keeps to the rules. */
function outsideSchemes(css: string): string {
  let rest = css
    .replace(/\{\{\/\*[\s\S]*?\*\/\}\}/g, "")
    .replace(/:root\s*\{\s*color-scheme:[^;{}]*;\s*(?:supported-color-schemes:[^;{}]*;\s*)?\}/, "");
  const at = rest.search(/@media\s*\(prefers-color-scheme:\s*dark\)\s*\{/);
  if (at >= 0) {
    let depth = 0;
    let end = rest.indexOf("{", at);
    for (; end < rest.length; end++) {
      if (rest[end] === "{") depth++;
      else if (rest[end] === "}" && --depth === 0) break;
    }
    rest = rest.slice(0, at) + rest.slice(end + 1);
  }
  return rest.trim();
}

/** The problems of `html`, a generated layout, each `<rule>: <what>`. */
export function checkFrame(html: string): string[] {
  const problems: string[] = [];
  let images = 0;
  let sheets = 0;
  let css = "";
  const seen = { html: false };
  const rewriter = new HTMLRewriter()
    .on("*", {
      element(el) {
        const tag = el.tagName.toLowerCase();
        if (!allowed.has(tag)) {
          const why = tag === "link" || tag === "script" || tag === "iframe" ? "external" : "tables";
          problems.push(`${why}: <${tag}> is no part of the layout; it is tables, with text and links in their cells`);
        }
        for (const [name, v] of el.attributes) {
          const attr = name.toLowerCase();
          if (attr.startsWith("on")) problems.push(`external: <${tag} ${attr}> runs a script; an email runs none`);
          if (attr === "style") {
            if (loads.test(v)) problems.push(`external: <${tag} style="${v}"> loads a resource`);
            if (layoutCss.test(v)) problems.push(`tables: <${tag} style="${v}"> lays out by CSS; lay out with tables`);
          }
          if (loading.has(attr) && !(tag === "img" && attr === "src")) {
            problems.push(`external: <${tag} ${attr}="${v}"> loads a resource`);
          }
          if (attr === "href" && /^\s*javascript:/i.test(v))
            problems.push(`external: <${tag} href="${v}"> runs a script`);
        }
        if (tag === "img") {
          images++;
          const src = el.getAttribute("src") ?? "";
          if (!action.test(src) || !/mail-logo/.test(el.getAttribute("class") ?? "")) {
            problems.push(
              `external: <img src="${src}"> is an image of the layout's own; the only image is the server's logo ` +
                "(class mail-logo-light or mail-logo-dark), whose URL the server fills in",
            );
          }
        }
        if (tag === "style") sheets++;
        if (tag === "html") {
          seen.html = true;
          for (const a of ["lang", "dir"]) {
            if ((el.getAttribute(a) ?? "").trim() === "")
              problems.push(`lang-dir: <html> has no ${a}, which the locale gives`);
          }
        }
      },
      comments(c) {
        if (loads.test(c.text) || /\ssrc\s*=/i.test(c.text))
          problems.push(`external: the comment <!--${c.text}--> loads a resource`);
      },
    })
    .on("style", {
      text(t) {
        css += t.text;
      },
    });
  rewriter.transform(html);
  if (!seen.html) problems.push("lang-dir: the layout has no <html>, whose lang and dir the locale gives");
  if (images > 2)
    problems.push(`external: the layout has ${images} images; the only ones are the logo and its dark version`);
  if (sheets > 1)
    problems.push(`inline-styles: the layout has ${sheets} stylesheets; the one in the head holds the dark colors`);
  if (loads.test(css)) problems.push("external: the stylesheet loads a resource");
  const rest = outsideSchemes(css);
  if (rest !== "") {
    problems.push(
      `inline-styles: the stylesheet holds ${JSON.stringify(rest.slice(0, 80))}; a look is an inline style, and the ` +
        "stylesheet holds only the color schemes and the dark colors",
    );
  }
  return problems;
}

if (import.meta.main) {
  let failed = false;
  for (const file of process.argv.slice(2)) {
    for (const p of checkFrame(await Bun.file(file).text())) {
      console.error(`${file}: ${p}`);
      failed = true;
    }
  }
  if (failed) process.exit(1);
}
