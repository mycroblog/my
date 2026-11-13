// SPDX-License-Identifier: AGPL-3.0-only

// Reads the HTML of a text that the server rendered and sanitized (a status's `content`, ADR-0019 §2) as data, which
// the content renderer draws with React. No HTML sink is involved: nothing in the text is ever handed to the browser
// as markup, so no dangerouslySetInnerHTML, DOMParser or Trusted Types policy is needed, and text is only ever text.
//
// The server sanitizes; this does not sanitize again but refuses to draw more than the server lets through. It keeps
// the elements and attributes of the server's allowlist (internal/adapter/htmlsanitizer) and the link schemes it
// allows, unwraps any other element to its children, and drops the elements whose contents the server drops (script,
// style, iframe…) with their contents. The server writes well-formed HTML that parses back to itself; anything else
// reads roughly as a browser would read it, and what this misreads becomes text.

/** The elements of the server's allowlist (internal/adapter/htmlsanitizer). */
const allowed = [
  "p",
  "br",
  "span",
  "a",
  "del",
  "s",
  "pre",
  "blockquote",
  "code",
  "b",
  "strong",
  "u",
  "i",
  "em",
  "ul",
  "ol",
  "li",
  "ruby",
  "rt",
  "rp",
] as const;

type ContentTag = (typeof allowed)[number];

const allowedTags: ReadonlySet<string> = new Set(allowed);

/** An element of a text, with only the attributes it may keep. */
export type ContentElement = {
  tag: ContentTag;
  attrs: Readonly<{
    lang?: string;
    /** a: a URL of an allowed scheme. */
    href?: string;
    /** a, span: the server's classes, such as mention, hashtag, invisible and ellipsis. */
    class?: string;
    /** a, span: "no" only. */
    translate?: "no";
    /** ol: an integer. */
    start?: string;
    /** ol: present or not. */
    reversed?: "";
    /** li: an integer. */
    value?: string;
  }>;
  children: ContentNode[];
};

/** A part of a text: text, which is only ever text, or an element. */
export type ContentNode = string | ContentElement;

/** The blocks of the allowlist; one of them closes an open paragraph, as in HTML. */
export const blockTags: ReadonlySet<string> = new Set(["p", "pre", "blockquote", "ul", "ol", "li"]);

/** The elements whose contents are dropped with them, as the server drops them. */
const droppedTags: ReadonlySet<string> = new Set([
  "script",
  "style",
  "template",
  "iframe",
  "frame",
  "frameset",
  "object",
  "embed",
  "noembed",
  "noframes",
  "noscript",
  "plaintext",
  "svg",
  "math",
  "xmp",
  "title",
  "textarea",
  "select",
  "head",
]);

/** The dropped elements whose contents are text up to their end tag, never markup. */
const rawTextTags: ReadonlySet<string> = new Set([
  "script",
  "style",
  "iframe",
  "noembed",
  "noframes",
  "noscript",
  "plaintext",
  "xmp",
  "title",
  "textarea",
]);

/** The elements that have no end tag. */
const voidTags: ReadonlySet<string> = new Set([
  "area",
  "base",
  "br",
  "col",
  "embed",
  "hr",
  "img",
  "input",
  "link",
  "meta",
  "param",
  "source",
  "track",
  "wbr",
]);

/** The schemes a link may have (the server's linkSchemes); any other link draws as its text. */
const linkSchemes: ReadonlySet<string> = new Set([
  "http",
  "https",
  "dat",
  "dweb",
  "ipfs",
  "ipns",
  "ssb",
  "gopher",
  "xmpp",
  "magnet",
  "gemini",
]);

const named: Readonly<Record<string, string>> = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", nbsp: " " };

const reference = /&(?:#(\d{1,8})|#[xX]([0-9a-fA-F]{1,6})|(amp|lt|gt|quot|apos|nbsp));/g;

/** Text with its character references read: the ones the server writes, and any by number. */
function decode(s: string): string {
  if (!s.includes("&")) return s;
  return s.replace(reference, (whole, dec: string | undefined, hex: string | undefined, name: string | undefined) => {
    if (name !== undefined) return named[name] ?? whole;
    const point = dec !== undefined ? Number.parseInt(dec, 10) : Number.parseInt(hex ?? "", 16);
    const valid = point > 0 && point <= 0x10ffff && (point < 0xd800 || point > 0xdfff);
    return valid ? String.fromCodePoint(point) : "�";
  });
}

const integer = /^-?[0-9]{1,9}$/;

/** The href of a link, as an absolute URL of an allowed scheme, or undefined. */
function linkHref(value: string | undefined): string | undefined {
  const v = value?.trim();
  if (v === undefined || /\s/.test(v)) return undefined;
  let url: URL;
  try {
    url = new URL(v);
  } catch {
    return undefined;
  }
  return linkSchemes.has(url.protocol.slice(0, -1)) ? url.href : undefined;
}

const attribute = /([^\s"'>/=]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+)))?/g;

/** What an element keeps of the attributes written in `source`. */
function attributesOf(tag: ContentTag, source: string): ContentElement["attrs"] {
  const raw = new Map<string, string>();
  for (const [, name = "", double, single, bare] of source.matchAll(attribute)) {
    const key = name.toLowerCase();
    if (!raw.has(key)) raw.set(key, decode(double ?? single ?? bare ?? ""));
  }
  const lang = raw.get("lang");
  const classes = raw.get("class");
  const translate = raw.get("translate") === "no" ? ("no" as const) : undefined;
  const start = raw.get("start");
  const value = raw.get("value");
  const href = linkHref(raw.get("href"));
  return {
    ...(lang !== undefined && { lang }),
    ...((tag === "a" || tag === "span") && {
      ...(classes !== undefined && { class: classes }),
      ...(translate && { translate }),
    }),
    ...(tag === "a" && href !== undefined && { href }),
    ...(tag === "ol" && {
      ...(start !== undefined && integer.test(start) && { start }),
      ...(raw.has("reversed") && { reversed: "" as const }),
    }),
    ...(tag === "li" && value !== undefined && integer.test(value) && { value }),
  };
}

/** An open element: one that is drawn, one that is unwrapped (its children go to its parent's), or one dropped. */
type Frame = { name: string; children: ContentNode[]; dropped?: true };

const tag = /<(\/?)([a-zA-Z][^\s/>]*)((?:[^>"']|"[^"]*"|'[^']*')*)>/y;
const bogus = /<(?:[!?]|\/[^a-zA-Z])/y;

/** The text's parts, from the HTML the server rendered and sanitized. */
export function parseContent(html: string): ContentNode[] {
  const root: ContentNode[] = [];
  const stack: Frame[] = [];
  const into = () => stack.at(-1)?.children ?? root;
  const text = (s: string) => {
    if (s === "") return;
    const children = into();
    const last = children.at(-1);
    if (typeof last === "string") children[children.length - 1] = last + s;
    else children.push(s);
  };
  // The open element named `name`, looked for no further out than a dropped element, whose insides close nothing
  // outside it.
  const open = (name: string) => {
    for (let i = stack.length - 1; i >= 0; i -= 1) {
      if (stack[i]?.name === name) return i;
      if (stack[i]?.dropped) return -1;
    }
    return -1;
  };
  const closeFrom = (i: number) => {
    if (i >= 0) stack.length = i;
  };
  const inside = (names: readonly string[]) => stack.some((frame) => names.includes(frame.name));

  let at = 0;
  while (at < html.length) {
    const lt = html.indexOf("<", at);
    if (lt < 0) {
      text(decode(html.slice(at)));
      break;
    }
    text(decode(html.slice(at, lt)));
    at = lt;
    if (html.startsWith("<!--", at)) {
      const end = html.indexOf("-->", at + 4);
      at = end < 0 ? html.length : end + 3;
      continue;
    }
    bogus.lastIndex = at;
    if (bogus.test(html)) {
      const end = html.indexOf(">", at);
      at = end < 0 ? html.length : end + 1;
      continue;
    }
    tag.lastIndex = at;
    const match = tag.exec(html);
    if (match === null) {
      text("<");
      at += 1;
      continue;
    }
    at = tag.lastIndex;
    const [, closing, written = "", source = ""] = match;
    const name = written.toLowerCase();
    if (closing === "/") {
      closeFrom(open(name));
      continue;
    }
    if (rawTextTags.has(name)) {
      const end = name === "plaintext" ? -1 : html.toLowerCase().indexOf(`</${name}`, at);
      const close = end < 0 ? -1 : html.indexOf(">", end);
      at = close < 0 ? html.length : close + 1;
      continue;
    }
    if (voidTags.has(name)) {
      if (name === "br") into().push({ tag: "br", attrs: {}, children: [] });
      continue;
    }
    if (droppedTags.has(name)) {
      stack.push({ name, children: [], dropped: true });
      continue;
    }
    if (blockTags.has(name)) closeFrom(open("p"));
    if (name === "li" && open("li") > Math.max(open("ul"), open("ol"))) closeFrom(open("li"));
    if (name === "a") closeFrom(open("a"));
    const drawn =
      allowedTags.has(name) &&
      (name !== "li" || inside(["ul", "ol"])) &&
      ((name !== "rt" && name !== "rp") || inside(["ruby"]));
    const attrs = drawn ? attributesOf(name as ContentTag, source) : {};
    if (!drawn || (name === "a" && attrs.href === undefined)) {
      stack.push({ name, children: into() });
      continue;
    }
    const element: ContentElement = { tag: name as ContentTag, attrs, children: [] };
    into().push(element);
    stack.push({ name, children: element.children });
    // The parser drops the first line break after <pre>, so the server writes one more there.
    if (name === "pre" && html[at] === "\n") at += 1;
  }
  return root;
}
