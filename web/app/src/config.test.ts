// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { parseConfig } from "./config";

const page = { origin: "http://localhost:5173", host: "localhost:5173" };

test("it reads the shell's data block", () => {
  const block = JSON.stringify({
    apiOrigin: "https://my.example",
    instanceName: "My Example",
    accountDomain: "my.example",
    defaultLocale: "ja",
    clientId: "web",
  });
  expect(parseConfig(block, page)).toEqual({
    apiOrigin: "https://my.example",
    instanceName: "My Example",
    defaultLocale: "ja",
    clientId: "web",
  });
});

test("without a data block, or a value in it, it takes the page's own origin and host and the default client", () => {
  const own = {
    apiOrigin: "http://localhost:5173",
    instanceName: "localhost:5173",
    defaultLocale: "en",
    clientId: "my-web",
  };
  expect(parseConfig(null, page)).toEqual(own);
  expect(parseConfig("{}", page)).toEqual(own);
});

test("a data block that is not an object fails", () => {
  expect(() => parseConfig("[1", page)).toThrow();
  expect(() => parseConfig("null", page)).toThrow("not an object");
});
