// SPDX-License-Identifier: AGPL-3.0-only

// The form that creates an event while whether the reader may create events is not known yet (ADR-0161 §3, FR-940):
// it says it is loading and offers no field, and shows the form once the server answered. A route cannot show this
// state (its loader holds the navigation), so the feature is drawn alone; the failed read and its Try again are the
// app's route tests' (web/app/src/events-organize.dom.test.tsx).

import { expect, onTestFinished, test } from "bun:test";
import { screen, waitFor } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { EventCreateScreen } from "./editor";

test("while whether the reader may create events is unknown, the form says it is loading, then shows its fields", async () => {
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  const { data, fake } = fakeData();
  const held = [
    fake.script("GET /instance", { hold: "request" }),
    fake.script("GET /accounts/verify_credentials", { hold: "request" }),
  ];
  renderFeature(
    data,
    <main>
      <EventCreateScreen onCreated={() => undefined} onBack={() => undefined} />
    </main>,
  );
  await waitFor(() => {
    expect(data.queryClient.isFetching()).toBeGreaterThan(0);
  });
  const shown = screen.getByRole("main");
  expect(shown.textContent).toMatch(/Loading/);
  expect(screen.queryAllByRole("textbox")).toHaveLength(0);
  expect(screen.queryAllByRole("alert")).toHaveLength(0);
  for (const script of held) script.release();
  expect(await screen.findByRole("textbox", { name: "Title" })).not.toBeNull();
});
