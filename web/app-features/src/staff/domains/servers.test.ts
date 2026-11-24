// SPDX-License-Identifier: AGPL-3.0-only

// The staff servers screen's address as the query of the remote servers (#3478): no filter lists every known server,
// "with problems" the servers deliveries failed to reach or marked unavailable, "unavailable" those marked so.

import { describe, expect, test } from "bun:test";
import { serverQueryOf } from "./servers";

describe("serverQueryOf", () => {
  test("asks for every known server when the address names no state", () => {
    const query = serverQueryOf({});
    expect(query.troubled).toBeUndefined();
    expect(query.state).toBeUndefined();
  });

  test("asks for the servers with problems for ?state=troubled, and names no state", () => {
    const query = serverQueryOf({ state: "troubled" });
    expect(query.troubled).toBe(true);
    expect(query.state).toBeUndefined();
  });

  test("asks for the servers marked unavailable for ?state=unavailable, without the servers with problems", () => {
    const query = serverQueryOf({ state: "unavailable" });
    expect(query.state).toBe("unavailable");
    expect(query.troubled).toBeUndefined();
  });
});
