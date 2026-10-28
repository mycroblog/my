// SPDX-License-Identifier: AGPL-3.0-only
// An ARIA snapshot for the jsdom component tests of @my/ui: the accessibility tree under an element in
// the YAML shape of Playwright's toMatchAriaSnapshot (web-client/README.md Y6), so a test can pin the structure
// a screen reader meets (roles, names, descriptions and states) without a browser.
//
//   - feed "Home" [busy]:
//     - article "Alice 2h" [posinset=1] [setsize=-1]:
//       - button "Show content" [expanded=false]
//
// Generic and presentational elements are flattened into their parent; text is listed only where it is
// not already an element's name. Hidden subtrees (`hidden`, `aria-hidden="true"`) are left out.
import { computeAccessibleDescription, computeAccessibleName, getRole } from "dom-accessibility-api";

/** Roles whose element contributes no node of its own. */
const flattened = new Set(["generic", "none", "presentation", "paragraph", "group", "time"]);

/** ARIA states and properties shown in brackets, by attribute. */
const states: readonly (readonly [string, string])[] = [
  ["aria-busy", "busy"],
  ["aria-expanded", "expanded"],
  ["aria-pressed", "pressed"],
  ["aria-posinset", "posinset"],
  ["aria-setsize", "setsize"],
  ["aria-live", "live"],
];

/**
 * The element's role. A <header> or <footer> inside sectioning content is no landmark (HTML-AAM), which
 * dom-accessibility-api does not know.
 */
function roleOf(element: Element): string | null {
  if ((element.localName === "header" || element.localName === "footer") && !element.hasAttribute("role")) {
    return element.parentElement?.closest("article, aside, main, nav, section") ? "generic" : getRole(element);
  }
  return getRole(element);
}

function isHidden(element: Element): boolean {
  return element.hasAttribute("hidden") || element.getAttribute("aria-hidden") === "true";
}

function quote(text: string): string {
  return JSON.stringify(text.replace(/\s+/g, " ").trim());
}

function describe(element: Element, role: string): string {
  const name = computeAccessibleName(element);
  const description = computeAccessibleDescription(element);
  const attributes = states.flatMap(([attribute, label]) => {
    const value = element.getAttribute(attribute);
    if (value === null || (attribute === "aria-busy" && value === "false")) return [];
    return attribute === "aria-busy" ? [`[${label}]`] : [`[${label}=${value}]`];
  });
  if (role === "heading") {
    const level =
      element.getAttribute("aria-level") ?? (/^h[1-6]$/.test(element.localName) ? element.localName.slice(1) : "2");
    attributes.unshift(`[level=${level}]`);
  }
  const lang = element.getAttribute("lang");
  if (lang !== null) attributes.push(`[lang=${lang}]`);
  const dir = element.getAttribute("dir");
  if (dir !== null) attributes.push(`[dir=${dir}]`);
  return [
    role,
    name === "" ? undefined : quote(name),
    description === "" ? undefined : `(${quote(description)})`,
    ...attributes,
  ]
    .filter((part) => part !== undefined)
    .join(" ");
}

function walk(node: Node, depth: number, lines: string[], parentNamed: boolean): void {
  for (const child of node.childNodes) {
    const indent = "  ".repeat(depth);
    if (child.nodeType === Node.TEXT_NODE) {
      const text = child.textContent?.replace(/\s+/g, " ").trim() ?? "";
      if (text !== "" && !parentNamed) lines.push(`${indent}- text: ${quote(text)}`);
      continue;
    }
    if (!(child instanceof Element) || isHidden(child)) continue;
    const role = roleOf(child);
    // Elements with user-content direction or language keep a node, so Y10 shows in the snapshot.
    const keepsNode = child.hasAttribute("dir") || child.hasAttribute("lang");
    if (role === null ? !keepsNode : flattened.has(role) && !keepsNode) {
      walk(child, depth, lines, parentNamed);
      continue;
    }
    const before = lines.length;
    lines.push(`${indent}- ${describe(child, role ?? "generic")}`);
    // Children of a named control are its name already (a button, a link, an image).
    const named =
      computeAccessibleName(child) !== "" &&
      ["button", "link", "img", "heading", "checkbox", "switch", "radio", "tab", "menuitem"].includes(role ?? "");
    walk(child, depth + 1, lines, named);
    if (lines.length > before + 1) lines[before] = `${lines[before] ?? ""}:`;
  }
}

/** The accessibility tree under `root` (not including `root` itself), one node per line. */
export function ariaSnapshot(root: Element): string {
  const lines: string[] = [];
  walk(root, 0, lines, false);
  return lines.join("\n");
}
