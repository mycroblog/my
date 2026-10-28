// SPDX-License-Identifier: AGPL-3.0-only
// The tokens of the text a user types, under the server's rules (ADR-0092 §4): mentions, hashtags and URLs
// as ParseText in internal/app/content/text.go finds them (FR-103), and `:shortcode:`s as ScanShortcodes in
// internal/domain/content/custom_emoji.go finds them (ADR-0051). Go and this file read the same vectors,
// pkg/testkit/fixtures/text/tokens.json, so a change to the rules has to change both. Offsets are UTF-16 code
// units, the offsets of a textarea's value and selection. Go's "word" characters are letters (L), marks (M),
// decimal digits (Nd only: unicode.IsDigit) and `_`, which is why the patterns use \p{Nd}, not \p{N}.

/** What a token is. */
type TextTokenKind = "mention" | "hashtag" | "url" | "shortcode";

/** A token of the text, [start, end) in UTF-16 code units. */
export interface TextToken {
  readonly kind: TextTokenKind;
  readonly start: number;
  readonly end: number;
  /** The token as typed: `@alice`, `@alice@example.org`, `#Go`, `https://example.com`, `:blobcat:`. */
  readonly text: string;
}

/** The trigger of the word being typed. */
type TokenTrigger = "@" | "#" | ":";

/** The word being typed at the cursor: from its trigger to the cursor, [start, end) in UTF-16 code units. */
export interface ActiveWord {
  readonly trigger: TokenTrigger;
  readonly start: number;
  readonly end: number;
  /** What follows the trigger up to the cursor: `ali` of `@ali`, `alice@exa` of `@alice@exa`. */
  readonly query: string;
}

const wordCharacter = /^[\p{L}\p{M}\p{Nd}_]$/u;
const letterOrMark = /^[\p{L}\p{M}]$/u;
/** A letter, a digit or a colon keeps a shortcode from starting or ending next to it (ScanShortcodes). */
const shortcodeBlocker = /^[\p{L}\p{Nd}:]$/u;
/** What keeps a `#` from starting a hashtag besides a word character (scanAt). */
const hashtagBlockers: readonly string[] = ["/", "=", ")", "&", "#"];
const asciiAlnum = /^[A-Za-z0-9]$/;
const usernameCharacter = /^[A-Za-z0-9_]$/;
const domainCharacter = /^[A-Za-z0-9.-]$/;
const shortcodeCharacter = /^[A-Za-z0-9_]$/;
/** The fewest characters between the colons of a shortcode (content.MinShortcodeLength). */
const minShortcodeLength = 2;

/** The code point that ends at `i`, or "" at the start. */
function codePointBefore(text: string, i: number): string {
  if (i <= 0) return "";
  const low = text.charCodeAt(i - 1);
  if (i >= 2 && low >= 0xdc00 && low <= 0xdfff) {
    const high = text.charCodeAt(i - 2);
    if (high >= 0xd800 && high <= 0xdbff) return text.slice(i - 2, i);
  }
  return text.charAt(i - 1);
}

/** The code point that starts at `i`, or "" at the end. */
function codePointAt(text: string, i: number): string {
  const cp = text.codePointAt(i);
  return cp === undefined ? "" : String.fromCodePoint(cp);
}

function isWord(c: string): boolean {
  return wordCharacter.test(c);
}

/** The end of the run of characters from `i` that `pattern` accepts, one UTF-16 unit each (ASCII patterns). */
function spanFrom(text: string, i: number, pattern: RegExp): number {
  let j = i;
  while (j < text.length && pattern.test(text.charAt(j))) j++;
  return j;
}

/** `@username` or `@username@domain` at `i` (scanMention): the end, or null. */
function scanMention(text: string, i: number): number | null {
  let end = spanFrom(text, i + 1, usernameCharacter);
  if (end === i + 1) return null;
  if (text.charAt(end) === "@") {
    let d = spanFrom(text, end + 1, domainCharacter);
    // A domain ends in a letter or digit: "@alice@example.org." keeps the final period as text.
    while (d > end + 1 && !asciiAlnum.test(text.charAt(d - 1))) d--;
    if (d > end + 1) end = d;
  }
  return end;
}

/** `#name` at `i` (scanHashtag): letters, marks, digits and `_` with at least one letter or mark. */
function scanHashtag(text: string, i: number): number | null {
  let j = i + 1;
  let hasLetter = false;
  for (let c = codePointAt(text, j); c !== "" && isWord(c); c = codePointAt(text, j)) {
    hasLetter ||= letterOrMark.test(c);
    j += c.length;
  }
  return hasLetter ? j : null;
}

/** Whether `c` may appear in an autolinked URL: printable ASCII but the characters RFC 3986 never allows (isURLByte). */
function isURLCharacter(c: string): boolean {
  const code = c.charCodeAt(0);
  return code > 0x20 && code < 0x7f && !'<>"`{}|\\^'.includes(c);
}

function count(s: string, c: string): number {
  return s.split(c).length - 1;
}

/** The length of `u` without trailing punctuation and unbalanced closing brackets (trimURL). */
function trimURL(u: string): number {
  let n = u.length;
  while (n > 0) {
    const c = u.charAt(n - 1);
    const head = u.slice(0, n);
    if (
      ".,:;!?'\"*".includes(c) ||
      (c === ")" && count(head, "(") < count(head, ")")) ||
      (c === "]" && count(head, "[") < count(head, "]"))
    ) {
      n--;
    } else {
      break;
    }
  }
  return n;
}

/** Whether every `%` of `s` starts an escape, and in a host only of a non-ASCII byte or `%25` (Go's url.unescape). */
function validEscapes(s: string, host = false): boolean {
  for (let k = s.indexOf("%"); k >= 0; k = s.indexOf("%", k + 3)) {
    const hex = s.slice(k + 1, k + 3);
    if (!/^[0-9A-Fa-f]{2}$/.test(hex)) return false;
    if (host && Number.parseInt(hex.charAt(0), 16) < 8 && hex !== "25") return false;
  }
  return true;
}

/** `:` followed by decimal digits only, or nothing (Go's validOptionalPort). */
function validPort(colonPort: string): boolean {
  return /^(:[0-9]*)?$/.test(colonPort);
}

const ipv4 = /^(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])){3}$/;

/** Whether `s` is an IPv6 address that is not an IPv4 one, as Go's netip.ParseAddr reads it (zones aside). */
function isIPv6(s: string): boolean {
  const halves = s.split("::");
  if (halves.length > 2) return false;
  const groups = halves.flatMap((h) => (h === "" ? [] : h.split(":")));
  let words = 0;
  for (const [k, g] of groups.entries()) {
    if (k === groups.length - 1 && g.includes(".")) {
      if (!ipv4.test(g)) return false;
      words += 2;
    } else if (/^[0-9A-Fa-f]{1,4}$/.test(g)) {
      words++;
    } else {
      return false;
    }
  }
  return halves.length === 2 ? words < 8 : words === 8;
}

/** Whether the authority of an http(s) URL has a host and no user information (Go's parseAuthority and parseHost). */
function validAuthority(authority: string): boolean {
  if (authority.includes("@")) return false;
  const open = authority.lastIndexOf("[");
  if (open > 0) return false;
  if (open === 0) {
    const close = authority.lastIndexOf("]");
    return close >= 0 && validPort(authority.slice(close + 1)) && isIPv6(authority.slice(1, close));
  }
  const colon = authority.indexOf(":");
  if (colon >= 0 && (colon !== authority.lastIndexOf(":") || !validPort(authority.slice(colon)))) return false;
  return (colon >= 0 ? colon : authority.length) > 0 && validEscapes(authority, true);
}

/** Whether `u` parses as a URL with a host and without user information, as url.Parse in scanURL decides. */
function linkableURL(u: string): boolean {
  const hash = u.indexOf("#");
  const beforeFragment = hash < 0 ? u : u.slice(0, hash);
  if (hash >= 0 && !validEscapes(u.slice(hash + 1))) return false;
  const query = beforeFragment.indexOf("?");
  const beforeQuery = query < 0 ? beforeFragment : beforeFragment.slice(0, query);
  const rest = beforeQuery.slice(beforeQuery.indexOf("://") + 3);
  const slash = rest.indexOf("/");
  return validAuthority(slash < 0 ? rest : rest.slice(0, slash)) && validEscapes(slash < 0 ? "" : rest.slice(slash));
}

/** An http or https URL at `i` (scanURL): the end, or null. */
function scanURL(text: string, i: number): number | null {
  const scheme = text.slice(i, i + 8).toLowerCase();
  if (!scheme.startsWith("http://") && !scheme.startsWith("https://")) return null;
  let j = i;
  while (j < text.length && isURLCharacter(text.charAt(j))) j++;
  const end = i + trimURL(text.slice(i, j));
  return linkableURL(text.slice(i, end)) ? end : null;
}

/** A mention, hashtag or URL at `i`, whose first character is `c` and which follows `prev` (scanAt). */
function scanAt(text: string, i: number, prev: string, c: string): TextToken | null {
  let kind: TextTokenKind;
  let end: number | null = null;
  if (c === "@" && !isWord(prev) && prev !== "/" && prev !== "=") {
    kind = "mention";
    end = scanMention(text, i);
  } else if (c === "#" && !isWord(prev) && !hashtagBlockers.includes(prev)) {
    kind = "hashtag";
    end = scanHashtag(text, i);
  } else if ((c === "h" || c === "H") && !isWord(prev)) {
    kind = "url";
    end = scanURL(text, i);
  } else {
    return null;
  }
  return end === null ? null : { kind, start: i, end, text: text.slice(i, end) };
}

/** The mentions, hashtags and URLs of `text`, in order (ParseText). */
function linkTokens(text: string): TextToken[] {
  const tokens: TextToken[] = [];
  let prev = "";
  for (let i = 0; i < text.length;) {
    const c = codePointAt(text, i);
    const token = scanAt(text, i, prev, c);
    if (token !== null) {
      tokens.push(token);
      i = token.end;
      prev = codePointBefore(text, i);
    } else {
      prev = c;
      i += c.length;
    }
  }
  return tokens;
}

/** The `:shortcode:`s of `text`, colons included, in order (ScanShortcodes). */
function shortcodeTokens(text: string): TextToken[] {
  const tokens: TextToken[] = [];
  for (let i = 0; i < text.length; i++) {
    if (text.charAt(i) !== ":" || shortcodeBlocker.test(codePointBefore(text, i))) continue;
    const j = spanFrom(text, i + 1, shortcodeCharacter);
    if (text.charAt(j) !== ":" || j - i - 1 < minShortcodeLength || shortcodeBlocker.test(codePointAt(text, j + 1)))
      continue;
    tokens.push({ kind: "shortcode", start: i, end: j + 1, text: text.slice(i, j + 1) });
    i = j; // the closing colon cannot open another shortcode: it follows a letter
  }
  return tokens;
}

/**
 * The tokens of `text`, in order and without overlaps: the mentions, hashtags and URLs the server links, and the
 * `:shortcode:`s outside them (a shortcode inside a URL is part of the URL). The rest of the text is plain.
 */
export function textTokens(text: string): TextToken[] {
  const links = linkTokens(text);
  const shortcodes = shortcodeTokens(text).filter((s) => !links.some((l) => s.start < l.end && l.start < s.end));
  return [...links, ...shortcodes].sort((a, b) => a.start - b.start);
}

/**
 * The trigger and the characters a word of each kind holds up to the cursor; the trigger follows what the server's
 * rules let it follow (scanAt and ScanShortcodes). Full-width `＠`, `＃` and `：` are not triggers.
 */
const triggerPatterns: readonly (readonly [TokenTrigger, RegExp])[] = [
  ["@", /(?<![\p{L}\p{M}\p{Nd}_/=])@[A-Za-z0-9_]*(?:@[A-Za-z0-9.-]*)?$/u],
  ["#", /(?<![\p{L}\p{M}\p{Nd}_/=)&#])#[\p{L}\p{M}\p{Nd}_]*$/u],
  [":", /(?<![\p{L}\p{Nd}:]):[A-Za-z0-9_]*$/u],
];

/**
 * The word being typed at `cursor` (a UTF-16 offset in `text`): an `@` mention, a `#` hashtag or a `:` shortcode
 * from its trigger to the cursor, or null when the cursor is not in one or the trigger is part of a URL. The query
 * may be empty or short; the caller decides how many characters open the completion (ADR-0092 §3).
 */
export function activeWord(text: string, cursor: number): ActiveWord | null {
  const before = text.slice(0, cursor);
  for (const [trigger, pattern] of triggerPatterns) {
    const match = pattern.exec(before);
    if (match === null) continue;
    const start = match.index;
    if (linkTokens(text).some((t) => t.kind === "url" && t.start < start && start < t.end)) return null;
    return { trigger, start, end: cursor, query: before.slice(start + 1) };
  }
  return null;
}
