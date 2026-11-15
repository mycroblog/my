// SPDX-License-Identifier: AGPL-3.0-only

// The HTML the server renders a local post's text as (internal/app/content/text.go, ParsedText.HTML), for the fake API
// (api.ts): a paragraph at each blank line, a line break at one, and its URLs, mentions and hashtags linked as
// Mastodon links them, a URL shown without its scheme and cut after 30 characters. Every mention resolves, to the
// profile page on the server of its handle. And the HTML of an article's body, for the Markdown the scenario's articles
// use.

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

/** A line of an article with its strong text and code, and its links (an image as the link of its alt text). */
function inline(line: string, host: string): string {
  return line
    .split(/(`[^`]+`)/)
    .map((part) => {
      if (part.startsWith("`") && part.endsWith("`") && part.length > 1)
        return `<code>${escape(part.slice(1, -1))}</code>`;
      let out = "";
      let at = 0;
      for (const m of part.matchAll(/!\[([^\]]*)\]\(([^)\s]+)\)|\*\*([^*]+)\*\*/g)) {
        const [whole, alt, url, strong] = m;
        out += linked(part.slice(at, m.index), host);
        out +=
          strong === undefined
            ? `<a href="${escape(url ?? "")}" ${rel}>${escape(alt ?? "")}</a>`
            : `<strong>${escape(strong)}</strong>`;
        at = m.index + whole.length;
      }
      return out + linked(part.slice(at), host);
    })
    .join("");
}

/** A Markdown table: its header row, its alignment row, its body rows. */
function table(lines: readonly string[], host: string): string {
  const cells = (line: string) =>
    line
      .replace(/^\||\|$/g, "")
      .split("|")
      .map((c) => c.trim());
  const aligns = cells(lines[1] ?? "").map((c) =>
    c.startsWith(":") && c.endsWith(":")
      ? ' align="center"'
      : c.endsWith(":")
        ? ' align="right"'
        : c.startsWith(":")
          ? ' align="left"'
          : "",
  );
  const row = (line: string, tag: "th" | "td") =>
    `<tr>${cells(line)
      .map((c, i) => `<${tag}${aligns[i] ?? ""}>${inline(c, host)}</${tag}>`)
      .join("")}</tr>`;
  const body = lines.slice(2).map((line) => row(line, "td"));
  return `<table><thead>${row(lines[0] ?? "", "th")}</thead>${body.length === 0 ? "" : `<tbody>${body.join("")}</tbody>`}</table>`;
}

/**
 * The HTML of an article's body, as the server renders the Markdown its author wrote (ADR-0160 §4), for the blocks the
 * scenario's articles use: headings a level down, paragraphs, lists, quotes, code, tables and rules, strong text and
 * code in a line, and an image as the link of its alt text, as one the server does not keep.
 */
export function articleHtml(markdown: string, host = "my.example"): string {
  return markdown
    .split(/\n{2,}/)
    .map((block) => {
      const lines = block.split("\n");
      const heading = /^(#{1,6}) (.*)$/.exec(block);
      if (heading !== null) {
        const level = Math.min((heading[1] ?? "").length + 1, 6);
        return `<h${String(level)}>${inline(heading[2] ?? "", host)}</h${String(level)}>`;
      }
      if (block === "---") return "<hr>";
      if (block.startsWith("```")) return `<pre><code>${escape(lines.slice(1, -1).join("\n"))}\n</code></pre>`;
      if (lines.every((l) => l.startsWith("- ")))
        return `<ul>${lines.map((l) => `<li>${inline(l.slice(2), host)}</li>`).join("")}</ul>`;
      if (lines.every((l) => l.startsWith("> ")))
        return `<blockquote><p>${lines.map((l) => inline(l.slice(2), host)).join("<br>")}</p></blockquote>`;
      if (lines.length > 1 && lines.every((l) => l.startsWith("|"))) return table(lines, host);
      return `<p>${lines.map((l) => inline(l, host)).join("<br>")}</p>`;
    })
    .join("");
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
