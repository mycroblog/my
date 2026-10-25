// SPDX-License-Identifier: MIT

/** The named character references that the server's sanitized HTML uses, and those common in text. */
const namedReferences: Readonly<Record<string, string>> = {
  amp: "&",
  lt: "<",
  gt: ">",
  quot: '"',
  apos: "'",
  nbsp: " ",
};

/** Decodes the character references of HTML text: numeric ones, and the named ones of namedReferences. */
function unescape(text: string): string {
  return text.replaceAll(/&(#[xX][0-9a-fA-F]+|#[0-9]+|[a-zA-Z]+);/g, (whole, ref: string) => {
    if (ref.startsWith("#")) {
      const code = ref[1] === "x" || ref[1] === "X" ? Number.parseInt(ref.slice(2), 16) : Number.parseInt(ref.slice(1), 10);
      return code > 0 && code <= 0x10ffff && !(code >= 0xd800 && code <= 0xdfff) ? String.fromCodePoint(code) : "�";
    }
    return namedReferences[ref] ?? whole;
  });
}

/** Whether c, after a '<', starts a tag, a closing tag, a comment, or a declaration. */
function opensTag(c: string | undefined): boolean {
  return c !== undefined && /^[/!?a-zA-Z]$/.test(c);
}

/** The lowercase name of an opening tag from what is between its < and >, "" for a closing tag, a comment, or a declaration. */
function tagName(tag: string): string {
  if (!/^\p{L}/u.test(tag)) return "";
  const end = tag.search(/[\s/]/);
  return (end >= 0 ? tag.slice(0, end) : tag).toLowerCase();
}

/**
 * Turns the HTML content of a post into plain text: the text of every element, mentions and links as they read
 * (`@alice`, `https://example.com/`), line breaks for <br> and between paragraphs, and character references decoded.
 * It is for reading a post, not for showing it: nothing of the markup survives.
 */
export function plainText(content: string): string {
  let out = "";
  let paragraphs = 0;
  let rest = content;
  while (rest !== "") {
    const i = rest.indexOf("<");
    if (i < 0) {
      out += unescape(rest);
      break;
    }
    out += unescape(rest.slice(0, i));
    rest = rest.slice(i);
    if (rest.length < 2 || !opensTag(rest[1])) {
      // A '<' that opens no tag, such as in "1 < 2", is text.
      out += "<";
      rest = rest.slice(1);
      continue;
    }
    const end = rest.indexOf(">");
    if (end < 0) {
      out += unescape(rest);
      break;
    }
    switch (tagName(rest.slice(1, end))) {
      case "br":
        out += "\n";
        break;
      case "p":
        if (paragraphs > 0) out += "\n\n";
        paragraphs++;
        break;
      default:
        break;
    }
    rest = rest.slice(end + 1);
  }
  return out.trim();
}
