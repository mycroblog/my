// SPDX-License-Identifier: AGPL-3.0-only

// What a note carries about the page (ADR-0168 §2), read from the DOM in the browser. An element in a same-origin
// iframe is read in its own document: its selector, its page coordinates and the state, whose viewport is the
// iframe's. The story is the one the element's window shows (story.ts).

import type { NoteInput, Rect, State } from "./note";
import { pageOf, storyOf } from "./story";

const pageRect = (win: Window, r: DOMRect): Rect => ({
  x: Math.round(r.left + win.scrollX),
  y: Math.round(r.top + win.scrollY),
  width: Math.round(r.width),
  height: Math.round(r.height),
});

/** A CSS selector that finds the element again after a reload: tag:nth-of-type steps up to an id or the body. */
function selectorOf(el: Element): string {
  const doc = el.ownerDocument;
  const steps: string[] = [];
  let node: Element | null = el;
  while (node && node !== doc.body && node !== doc.documentElement) {
    if (node.id) {
      steps.unshift(`#${CSS.escape(node.id)}`);
      break;
    }
    const tag = node.tagName.toLowerCase();
    const parent: Element | null = node.parentElement;
    const same = parent ? Array.from(parent.children).filter((c) => c.tagName === node?.tagName) : [];
    steps.unshift(same.length > 1 ? `${tag}:nth-of-type(${same.indexOf(node) + 1})` : tag);
    node = parent;
  }
  return steps.length === 0 ? "body" : steps.join(" > ");
}

// In an iframe, screen.orientation tells of the real screen, so the orientation is the iframe viewport's, as the
// orientation media query reads it.
function orientationOf(win: Window): string | null {
  if (win !== win.top) return win.matchMedia("(orientation: portrait)").matches ? "portrait" : "landscape";
  return typeof screen.orientation === "undefined" ? null : screen.orientation.type;
}

function stateOf(win: Window): State {
  const html = win.document.documentElement;
  return {
    page: pageOf(win),
    route: null,
    viewport: { width: win.innerWidth, height: win.innerHeight, devicePixelRatio: win.devicePixelRatio },
    theme: {
      page: html.getAttribute("data-theme"),
      system: win.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
    },
    locale: html.lang || navigator.language,
    direction: win.getComputedStyle(html).direction,
    orientation: orientationOf(win),
    account: null,
  };
}

const implicitRoles: Record<string, string> = {
  a: "link",
  article: "article",
  aside: "complementary",
  button: "button",
  dialog: "dialog",
  footer: "contentinfo",
  form: "form",
  h1: "heading",
  h2: "heading",
  h3: "heading",
  h4: "heading",
  h5: "heading",
  h6: "heading",
  header: "banner",
  img: "img",
  input: "textbox",
  li: "listitem",
  main: "main",
  nav: "navigation",
  ol: "list",
  p: "paragraph",
  section: "region",
  select: "combobox",
  table: "table",
  textarea: "textbox",
  ul: "list",
};

function roleOf(el: Element): string | null {
  const explicit = el.getAttribute("role");
  if (explicit) return explicit;
  const tag = el.tagName.toLowerCase();
  if (tag === "input") {
    const type = (el as HTMLInputElement).type;
    if (type === "checkbox" || type === "radio") return type;
    if (type === "button" || type === "submit") return "button";
  }
  return implicitRoles[tag] ?? null;
}

const clip = (s: string, n = 80) => {
  const t = s.replace(/\s+/g, " ").trim();
  return t.length > n ? `${t.slice(0, n - 1)}…` : t;
};

function nameOf(el: Element): string {
  const label = el.getAttribute("aria-label");
  if (label) return label;
  const labelledBy = el.getAttribute("aria-labelledby");
  if (labelledBy) {
    return labelledBy
      .split(/\s+/)
      .map((id) => el.ownerDocument.getElementById(id)?.textContent ?? "")
      .join(" ");
  }
  return el.getAttribute("alt") ?? el.getAttribute("title") ?? "";
}

// The roles whose name is their text, as a heading's or a button's.
const namedByContent = new Set(["button", "checkbox", "heading", "link", "radio"]);

/**
 * An outline of the accessibility tree under the elements, one line per node with a role and per run of text: what
 * the DOM tells of the roles and names, not the browser's computed tree.
 */
function accessibilityOf(roots: Element[], limit = 40): string {
  const lines: string[] = [];
  const walk = (child: Node, depth: number) => {
    if (lines.length >= limit) return;
    const indent = "  ".repeat(depth);
    if (child.nodeType === Node.TEXT_NODE) {
      const text = clip(child.textContent ?? "");
      if (!text) return;
      // Runs of text side by side (a label and its value) make one line.
      const last = lines.at(-1);
      if (last?.startsWith(`${indent}text "`)) lines[lines.length - 1] = `${last.slice(0, -1)} ${text}"`;
      else lines.push(`${indent}text "${text}"`);
      return;
    }
    // Not instanceof Element: an element of an iframe belongs to the iframe's realm.
    if (child.nodeType !== Node.ELEMENT_NODE) return;
    const node = child as Element;
    if (node.getAttribute("aria-hidden") === "true") return;
    const role = roleOf(node);
    if (!role) {
      for (const c of Array.from(node.childNodes)) walk(c, depth);
      return;
    }
    const byContent = namedByContent.has(role);
    const name = clip(nameOf(node) || (byContent ? node.textContent : ""));
    const level = /^h[1-6]$/i.test(node.tagName) ? ` [level=${node.tagName.slice(1)}]` : "";
    lines.push(`${indent}${role}${name ? ` "${name}"` : ""}${level}`);
    if (byContent) return;
    for (const c of Array.from(node.childNodes)) walk(c, depth + 1);
  };
  for (const root of roots) walk(root, 0);
  return lines.join("\n");
}

const overlaps = (a: DOMRect, b: Rect) =>
  a.width > 0 && a.right > b.x && a.left < b.x + b.width && a.bottom > b.y && a.top < b.y + b.height;
const covers = (a: DOMRect, b: Rect) =>
  a.left <= b.x && a.right >= b.x + b.width && a.top <= b.y && a.bottom >= b.y + b.height;

/**
 * The note on an element, or on an area whose centre is on the element; the area is in the client coordinates of the
 * element's document.
 */
export function noteOf(text: string, el: Element, area: Rect | null): NoteInput {
  const doc = el.ownerDocument;
  const win = doc.defaultView ?? window;
  // The elements the area touches, without those around the whole area: an area over two swatches is about them,
  // not about the page that holds them.
  const inArea = area
    ? Array.from(doc.querySelectorAll("[data-source]")).filter((e) => {
        const r = e.getBoundingClientRect();
        return overlaps(r, area) && !covers(r, area);
      })
    : [];
  const sources = [...new Set(inArea.map((e) => e.getAttribute("data-source") ?? ""))].slice(0, 20);
  // The outermost elements in the area, whose subtrees the outline covers.
  const tops = inArea.filter((e) => !inArea.some((o) => o !== e && o.contains(e)));
  return {
    text,
    target: {
      tag: el.tagName.toLowerCase(),
      selector: selectorOf(el),
      text: clip(el.textContent),
      rect: pageRect(win, el.getBoundingClientRect()),
      area: area ? pageRect(win, new DOMRect(area.x, area.y, area.width, area.height)) : null,
    },
    source: el.closest("[data-source]")?.getAttribute("data-source") ?? null,
    sources,
    state: stateOf(win),
    story: storyOf(win),
    accessibility: accessibilityOf(area && tops.length > 0 ? tops : [el]),
  };
}
