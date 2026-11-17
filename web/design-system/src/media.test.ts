// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { playTime } from "./media";

test("a length reads as minutes and seconds, with hours when it has them", () => {
  expect(playTime(7)).toBe("0:07");
  expect(playTime(192)).toBe("3:12");
  expect(playTime(3725)).toBe("1:02:05");
});
