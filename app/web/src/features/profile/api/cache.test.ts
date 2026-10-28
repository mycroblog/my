// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import {
  imageFixture,
  otherAccountFixture,
  type Schemas,
  statusFixture,
  statusPage,
  viewerFixture,
  viewerStatusFixture,
} from "../../../test/fixtures";
import { updateAccounts } from "./cache";

// The account fields of the viewer; the rest of their own answer plays no part.
const viewer: Schemas["Account"] = viewerFixture;
const saved = {
  ...viewer,
  display_name: "Alice in Chains",
  note: "<p>Hello</p>",
  avatar: imageFixture("https://my.example/media/new.png"),
  followers_count: 3,
  following_count: 2,
};

describe("updateAccounts", () => {
  test("sets the profile of the account's own answer, with its follow counts", () => {
    const lookup = {
      ...viewer,
      avatar: imageFixture("https://my.example/media/old.png"),
      header: imageFixture("https://my.example/media/header.png"),
      followers_count: 1,
      following_count: 1,
    };
    const next = updateAccounts(lookup, saved);
    expect(next).toEqual({
      ...lookup,
      display_name: "Alice in Chains",
      note: "<p>Hello</p>",
      avatar: imageFixture("https://my.example/media/new.png"),
      header: undefined,
    });
    // The header image that the change removed is left out, as the server leaves it out.
    expect(next).not.toHaveProperty("header");
    // The counts stay those of the answer: a change of the profile does not move them.
    expect(next).toMatchObject({ followers_count: 1, following_count: 1 });
  });

  test("sets the author of the viewer's posts in the pages of a timeline, in boosts and in notifications, and keeps the rest", () => {
    const mine = viewerStatusFixture();
    const other = statusFixture("0192f0c4-7a8e-7000-8000-0000000000c2");
    const boost = { ...other, id: "0192f0c4-7a8e-7000-8000-0000000000c3", boost: mine };
    const data = { pages: [statusPage([other, mine]), statusPage([boost])], pageParams: [undefined, "page-2"] };
    const next = updateAccounts(data, saved) as typeof data;
    expect(next.pages[0]?.items[0]).toBe(other);
    expect(next.pages[0]?.items[1]?.account).toMatchObject({ display_name: "Alice in Chains", avatar: saved.avatar });
    // An embedded account has no follow counts, and gets none.
    expect(next.pages[0]?.items[1]?.account).not.toHaveProperty("followers_count");
    expect(next.pages[1]?.items[0]?.account).toBe(other.account);
    expect(next.pages[1]?.items[0]?.boost?.account.display_name).toBe("Alice in Chains");
    expect(next.pageParams).toBe(data.pageParams);

    const notifications = {
      items: [
        { id: "n1", type: "follow", account: viewer },
        { id: "n2", type: "favourite", account: otherAccountFixture, status: mine },
      ],
    };
    const updated = updateAccounts(notifications, saved) as typeof notifications;
    expect(updated.items[0]?.account.display_name).toBe("Alice in Chains");
    expect(updated.items[1]?.account).toBe(otherAccountFixture);
    expect(updated.items[1]?.status?.account.display_name).toBe("Alice in Chains");
  });

  test("leaves the mentions of the account, and an answer without the account, as they are", () => {
    const other = statusFixture("0192f0c4-7a8e-7000-8000-0000000000c2");
    const page = statusPage([other]);
    // The post mentions the viewer, but a mention is not the account's profile.
    expect(updateAccounts(page, saved)).toBe(page);
    const unchanged = { ...viewer, ...saved, followers_count: undefined };
    expect(updateAccounts(unchanged, saved)).toBe(unchanged);
  });
});
