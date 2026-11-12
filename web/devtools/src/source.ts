// SPDX-License-Identifier: AGPL-3.0-only

// Adds data-source="file:line" to each host element of a JSX file, so that a note knows the source of the element
// it is on (ADR-0168 §2). Only the dev server runs it.

import MagicString from "magic-string";
import { parseSync } from "vite";

interface Node {
  type: string;
  start: number;
  end: number;
  [key: string]: unknown;
}

const isNode = (v: unknown): v is Node =>
  typeof v === "object" && v !== null && typeof (v as { type?: unknown }).type === "string";

// The opening elements of host elements (<div>, <h1>, <my-element>), the ones that reach the DOM.
function hostElements(node: unknown, out: Node[]): void {
  if (typeof node !== "object" || node === null) return;
  if (isNode(node) && node.type === "JSXOpeningElement") {
    const name = node["name"];
    if (isNode(name) && name.type === "JSXIdentifier" && /^[a-z]/.test(String(name["name"]))) out.push(node);
  }
  for (const value of Object.values(node)) hostElements(value, out);
}

function hasSource(element: Node): boolean {
  const attributes = element["attributes"];
  if (!Array.isArray(attributes)) return false;
  return attributes.some(
    (a) => isNode(a) && a.type === "JSXAttribute" && isNode(a["name"]) && a["name"]["name"] === "data-source",
  );
}

/**
 * Returns the code with data-source="<file>:<line>" on each host element that has none, and its source map, or null
 * when the file has no host element. `file` is the path the attribute names, relative to the workspace root.
 */
export function addSources(
  code: string,
  id: string,
  file: string,
): { code: string; map: ReturnType<MagicString["generateMap"]> } | null {
  const lang = id.endsWith(".jsx") ? "jsx" : "tsx";
  const { program } = parseSync(id, code, { lang });
  const elements: Node[] = [];
  hostElements(program, elements);
  if (elements.length === 0) return null;
  const lineStarts = [0];
  for (let i = 0; i < code.length; i++) if (code[i] === "\n") lineStarts.push(i + 1);
  const lineOf = (offset: number) => {
    let lo = 0;
    let hi = lineStarts.length - 1;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if ((lineStarts[mid] ?? 0) <= offset) lo = mid;
      else hi = mid - 1;
    }
    return lo + 1;
  };
  const s = new MagicString(code);
  for (const element of elements) {
    if (hasSource(element)) continue;
    const name = element["name"] as Node;
    s.appendLeft(name.end, ` data-source="${file}:${lineOf(element.start)}"`);
  }
  return { code: s.toString(), map: s.generateMap({ hires: true, source: id, includeContent: true }) };
}
