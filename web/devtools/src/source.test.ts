// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { addSources } from "./source";

describe("addSources", () => {
  test("adds the file and line to each host element", () => {
    const code = `export const A = () => (\n  <section>\n    <h1 className="x">é</h1>\n    <my-el />\n  </section>\n);\n`;
    const out = addSources(code, "/w/web/a/src/A.tsx", "web/a/src/A.tsx");
    expect(out?.code).toBe(
      `export const A = () => (\n  <section data-source="web/a/src/A.tsx:2">\n` +
        `    <h1 data-source="web/a/src/A.tsx:3" className="x">é</h1>\n` +
        `    <my-el data-source="web/a/src/A.tsx:4" />\n  </section>\n);\n`,
    );
    expect(out?.map.sources).toEqual(["/w/web/a/src/A.tsx"]);
  });

  test("leaves components, fragments, and an element with a source alone", () => {
    const code = `const B = <><Card.Body /><Card /><p data-source="kept" /></>;\n`;
    expect(addSources(code, "B.tsx", "B.tsx")?.code).toBe(code);
  });

  test("returns null for a file without host elements", () => {
    expect(addSources(`const C = <Card />;\n`, "C.tsx", "C.tsx")).toBeNull();
  });
});
