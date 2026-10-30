// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test } from "bun:test";
import { literalText, parseMessage } from "./icu";

test("literalText keeps the text of every branch and drops arguments and ICU syntax", () => {
  const text = literalText(parseMessage("理由: {reason}、{count, plural, other {残り # 文字}}<b>強調</b>"));
  expect(text).toContain("理由: ");
  expect(text).toContain("残り");
  expect(text).toContain("強調");
  expect(text).not.toContain(",");
  expect(text).not.toContain("reason");
});
