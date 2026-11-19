// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { stringsIn } from "./locale";
import { sentences } from "./supporter-words";

test("sentences of a question follow each other with a space in en and with none in ja", () => {
  const en = stringsIn("en").t;
  const ja = stringsIn("ja").t;
  const words = { plan: "Supporter", interval: "month" } as const;
  expect(
    sentences("en", [
      en("settings.supporter.cancel.text", { date: "Dec 1, 2026" }),
      en("settings.supporter.cancel.scheduled", words),
    ]),
  ).toBe(
    `${en("settings.supporter.cancel.text", { date: "Dec 1, 2026" })} ${en("settings.supporter.cancel.scheduled", words)}`,
  );
  const joined = sentences("ja-JP", [
    ja("settings.supporter.cancel.text", { date: "2026/12/01" }),
    ja("settings.supporter.cancel.scheduled", words),
  ]);
  expect(joined).toBe(
    `${ja("settings.supporter.cancel.text", { date: "2026/12/01" })}${ja("settings.supporter.cancel.scheduled", words)}`,
  );
  expect(joined).not.toContain("。 ");
  expect(sentences("zh-TW", ["一。", "二。"])).toBe("一。二。");
  expect(sentences("de", ["Eins.", "Zwei."])).toBe("Eins. Zwei.");
});
