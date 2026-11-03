// SPDX-License-Identifier: AGPL-3.0-only
// An action on a list of the settings puts the relationship it answers into every cached answer of
// GET /accounts/relationships that holds the account, so that a profile opened next shows it at once
// (web-client/README.md S1). A failure here fails the action itself: the row stays on the list.
import { QueryClient } from "@tanstack/react-query";
import { describe, expect, test } from "vitest";
import { relationshipFixture } from "../../test/fixtures";
import { cacheListChange } from "./api/relationships";

const bobId = "0192f0c4-7a8e-7000-8000-0000000000b1";
const carolId = "0192f0c4-7a8e-7000-8000-0000000000c1";
const viewerId = "0192f0c4-7a8e-7000-8000-0000000000d1";

function cachedRelationships(queryClient: QueryClient, ids: string[]) {
  const key = ["get", "/accounts/relationships", { params: { query: { id: ids } } }];
  queryClient.setQueryData(key, { items: ids.map((id) => relationshipFixture(id)) });
  return key;
}

describe.each(["accept", "reject", "unblock", "unmute"] as const)("the list change %s", (change) => {
  test("replaces the account's relationship in the cached answers that hold it, and only there", async () => {
    const queryClient = new QueryClient();
    const withBob = cachedRelationships(queryClient, [bobId, carolId]);
    const withoutBob = cachedRelationships(queryClient, [carolId]);
    const changed = { ...relationshipFixture(bobId), followed_by: true };

    await cacheListChange(queryClient, change, changed, viewerId);

    expect(queryClient.getQueryData(withBob)).toEqual({ items: [changed, relationshipFixture(carolId)] });
    expect(queryClient.getQueryData(withoutBob)).toEqual({ items: [relationshipFixture(carolId)] });
  });
});
