// SPDX-License-Identifier: AGPL-3.0-only

// Where a refusal of a plan stands, and which roles a new plan is offered (FR-912): a new plan's form has no position
// nor permissions, so what the server finds wrong with them stands under the role, where a plan's form puts them under
// their own fields; a 404 on a plan's form is the plan's, which the form says beside its button. The roles offered are
// those with member permissions alone, under every staff role, and on no plan yet; the tips' amounts keep "1,000" one.

import { expect, test } from "bun:test";
import type { AdminRole } from "@my/app-data";
import { tipAmountsOf } from "./parts";
import { inputOfReason, planRoles } from "./plans";

test.each([
  ["role_position", "role", "position"],
  ["order_position", "role", "position"],
  ["role_permissions", "role", "permissions"],
  ["order_permissions", "role", "permissions"],
  ["role_taken", "role", undefined],
  ["unknown-role", "role", undefined],
  ["name", "name", "name"],
] as const)("a refusal for %s stands under %s on a new plan, under %s on a plan", (reason, creating, editing) => {
  expect(inputOfReason(reason, false)).toBe(creating);
  expect(inputOfReason(reason, true)).toBe(editing);
});

const role = (key: string, position: number, permissions: AdminRole["permissions"] = []): AdminRole => ({
  id: key,
  key,
  position,
  permissions,
});

test("a new plan is offered the roles of member permissions under every staff role and on no plan", () => {
  const roles = [
    role("admin", 1000, ["manage_settings"]),
    role("helper", 60, ["manage_reports"]),
    role("moderator", 100, ["manage_reports"]),
    role("gold", 50, ["view_finance"]),
    role("silver", 40, ["create_custom_emojis"]),
    role("taken", 30),
    role("above", 70, ["view_finance"]),
  ];
  expect(planRoles(roles, new Set(["taken"])).map((r) => r.key)).toEqual(["gold", "silver"]);
});

test("the tips' amounts are one a line or apart by a semicolon or 、, a thousands separator kept inside one", () => {
  expect(tipAmountsOf("1,000\n500、3 000; 10,000\n\n")).toEqual(["1000", "500", "3000", "10000"]);
});
