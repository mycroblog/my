// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so it cannot tell an alert that can
// take the focus from one that cannot (ADR-0171 §4).

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./button";
import { FormActions, FormAlert } from "./form-page";

test("a form's alert takes the focus a page gives it, and Tab goes on to the button after it", async () => {
  const user = userEvent.setup();
  render(
    <FormActions>
      <FormAlert focus>The password is wrong.</FormAlert>
      <Button kind="primary" type="submit">
        Sign in
      </Button>
    </FormActions>,
  );
  const alert = screen.getByRole("alert");

  alert.focus();
  expect(document.activeElement).toBe(alert);
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Sign in" }));
});

test("a form's alert is no stop of Tab by itself", async () => {
  const user = userEvent.setup();
  render(
    <FormActions>
      <FormAlert focus>The password is wrong.</FormAlert>
      <Button kind="primary" type="submit">
        Sign in
      </Button>
    </FormActions>,
  );

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Sign in" }));
});
