// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { confirmationFields, type Confirmation } from "./confirmation";

const passkey = { confirmation_token: "t", credential: { id: "c" } };

describe("confirmationFields", () => {
  const table: readonly (readonly [string, Confirmation, "password" | "current_password" | undefined, object])[] = [
    [
      "a password, by default as `password`",
      { kind: "password", password: "hunter22" },
      undefined,
      { password: "hunter22" },
    ],
    ["a password, as `password`", { kind: "password", password: "hunter22" }, "password", { password: "hunter22" }],
    [
      "a password, as `current_password` where the operation names it so",
      { kind: "password", password: "hunter22" },
      "current_password",
      { current_password: "hunter22" },
    ],
    ["a passkey, whatever the password field", { kind: "passkey", passkey }, "current_password", { passkey }],
    ["a linked provider's confirmation", { kind: "external", token: "x" }, undefined, { external_confirmation: "x" }],
  ];

  for (const [name, confirmation, field, fields] of table)
    test(`maps ${name}`, () => {
      expect(confirmationFields(confirmation, field)).toEqual(fields);
    });
});
