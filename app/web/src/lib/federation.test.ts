// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { ApiError } from "./errors";
import { federationReachOf, isFederationRestricted } from "./federation";

describe("federationReachOf", () => {
  // ADR-0076, "流れの分類": follow_out and ingest pass in inbound and both; publish passes in outbound and both.
  test.each([
    ["off", false, false],
    ["inbound", true, false],
    ["outbound", false, true],
    ["both", true, true],
  ] as const)("%s: follows remote %s, reaches remote %s", (mode, followsRemote, reachesRemote) => {
    expect(federationReachOf(mode)).toEqual({ mode, followsRemote, reachesRemote });
  });
});

describe("isFederationRestricted", () => {
  test("is the 403 of the federation-restricted problem only", () => {
    expect(
      isFederationRestricted(
        new ApiError({ type: "/api/v1/problems/federation-restricted", title: "Restricted", status: 403 }),
      ),
    ).toBe(true);
    expect(isFederationRestricted(new ApiError({ type: "about:blank", title: "Forbidden", status: 403 }))).toBe(false);
    expect(isFederationRestricted(new Error("network"))).toBe(false);
  });
});
