// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { factorOf } from "./topics";

describe("factorOf", () => {
  test("shows a setting the server keeps in minutes or days as it is", () => {
    expect(factorOf("scheduled_post_min_lead_minutes")).toBe(1);
    expect(factorOf("appeal_window_days")).toBe(1);
  });

  test("shows seconds as hours or minutes, and bytes as MiB", () => {
    expect(factorOf("member_invite_max_lifetime_seconds")).toBe(3600);
    expect(factorOf("remote_media_cache_ttl_seconds")).toBe(60);
    expect(factorOf("remote_media_cache_max_bytes")).toBe(1_048_576);
  });
});
