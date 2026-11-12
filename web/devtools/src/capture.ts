// SPDX-License-Identifier: AGPL-3.0-only

// What a note carries about the page (ADR-0168 §2), read from the DOM in the browser.

import type { NoteInput, Rect, State } from "./note";

const pageRect = (r: DOMRect): Rect => ({
  x: Math.round(r.left + window.scrollX),
  y: Math.round(r.top + window.scrollY),
  width: Math.round(r.width),
  height: Math.round(r.height),
});

/** A CSS selector that finds the element again after a reload: tag:nth-of-type steps up to an id or the body. */
function selectorOf(el: Element): string {
  const steps: string[] = [];
  let node: Element | null = el;
  while (node && node !== document.body && node !== document.documentElement) {
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

function stateOf(): State {
  const html = document.documentElement;
  return {
    page: location.pathname + location.search + location.hash,
    route: null,
    viewport: { width: window.innerWidth, height: window.innerHeight, devicePixelRatio: window.devicePixelRatio },
    theme: {
      page: html.getAttribute("data-theme"),
      system: window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
    },
    locale: html.lang || navigator.language,
    direction: getComputedStyle(html).direction,
    orientation: typeof screen.orientation === "undefined" ? null : screen.orientation.type,
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
      .map((id) => document.getElementById(id)?.textContent ?? "")
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
  const walk = (node: Node, depth: number) => {
    if (lines.length >= limit) return;
    const indent = "  ".repeat(depth);
    if (node.nodeType === Node.TEXT_NODE) {
      const text = clip(node.textContent ?? "");
      if (!text) return;
      // Runs of text side by side (a label and its value) make one line.
      const last = lines.at(-1);
      if (last?.startsWith(`${indent}text "`)) lines[lines.length - 1] = `${last.slice(0, -1)} ${text}"`;
      else lines.push(`${indent}text "${text}"`);
      return;
    }
    if (!(node instanceof Element) || node.getAttribute("aria-hidden") === "true") return;
    const role = roleOf(node);
    if (!role) {
      for (const child of Array.from(node.childNodes)) walk(child, depth);
      return;
    }
    const byContent = namedByContent.has(role);
    const name = clip(nameOf(node) || (byContent ? node.textContent : ""));
    const level = /^h[1-6]$/i.test(node.tagName) ? ` [level=${node.tagName.slice(1)}]` : "";
    lines.push(`${indent}${role}${name ? ` "${name}"` : ""}${level}`);
    if (byContent) return;
    for (const child of Array.from(node.childNodes)) walk(child, depth + 1);
  };
  for (const root of roots) walk(root, 0);
  return lines.join("\n");
}

const overlaps = (a: DOMRect, b: Rect) =>
  a.width > 0 && a.right > b.x && a.left < b.x + b.width && a.bottom > b.y && a.top < b.y + b.height;
const covers = (a: DOMRect, b: Rect) =>
  a.left <= b.x && a.right >= b.x + b.width && a.top <= b.y && a.bottom >= b.y + b.height;

/** The note on an element, or on an area (client coordinates) whose centre is on the element. */
export function noteOf(text: string, el: Element, area: Rect | null): NoteInput {
  // The elements the area touches, without those around the whole area: an area over two swatches is about them,
  // not about the page that holds them.
  const inArea = area
    ? Array.from(document.querySelectorAll("[data-source]")).filter((e) => {
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
      rect: pageRect(el.getBoundingClientRect()),
      area: area ? pageRect(new DOMRect(area.x, area.y, area.width, area.height)) : null,
    },
    source: el.closest("[data-source]")?.getAttribute("data-source") ?? null,
    sources,
    state: stateOf(),
    reproduction: { scenario: null, step: null },
    accessibility: accessibilityOf(area && tops.length > 0 ? tops : [el]),
  };
}
