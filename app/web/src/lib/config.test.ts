// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { readRuntimeConfig, RuntimeConfigError } from "./config";

const valid = {
  apiOrigin: "https://my.example",
  instanceName: "my example",
  accountDomain: "my.example",
  defaultLocale: "ja",
};

/** A document with the shell's data block holding `text`. */
function shellWith(text: string, type = "application/json"): Document {
  const doc = document.implementation.createHTMLDocument();
  const script = doc.createElement("script");
  script.type = type;
  script.id = "config";
  script.text = text;
  doc.head.append(script);
  return doc;
}

describe("readRuntimeConfig", () => {
  test("returns the config in the shell's data block", () => {
    expect(readRuntimeConfig(shellWith(JSON.stringify(valid)))).toEqual(valid);
  });

  test("reads the instance's own brand images", () => {
    const brand = { ...valid, icon: { url: "/brand/r1/icon.svg", width: 64, height: 64 } };
    expect(readRuntimeConfig(shellWith(JSON.stringify(brand)))).toEqual(brand);
  });

  test("ignores fields the client does not know", () => {
    expect(readRuntimeConfig(shellWith(JSON.stringify({ ...valid, future: true })))).toEqual(valid);
  });

  test.each([
    ["no data block", document.implementation.createHTMLDocument()],
    ["a data block of another type", shellWith(JSON.stringify(valid), "text/plain")],
    ["text that is not JSON", shellWith("{")],
    ["a missing field", shellWith(JSON.stringify({ ...valid, instanceName: undefined }))],
    ["an empty name", shellWith(JSON.stringify({ ...valid, instanceName: "" }))],
    [
      "a brand image elsewhere than under /brand/",
      shellWith(JSON.stringify({ ...valid, logo: { url: "https://elsewhere.example/logo.svg", width: 4, height: 1 } })),
    ],
    [
      "an API origin that is not an http(s) URL",
      shellWith(JSON.stringify({ ...valid, apiOrigin: "javascript:alert(1)" })),
    ],
  ])("throws a RuntimeConfigError for %s", (_, doc) => {
    expect(() => readRuntimeConfig(doc)).toThrow(RuntimeConfigError);
  });
});
