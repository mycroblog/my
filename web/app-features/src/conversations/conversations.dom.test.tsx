// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages as a feature: a conversation that is not the reader's says so.

import { expect, test } from "bun:test";
import { screen } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { ConversationView, loadConversation } from "./conversations";

test("a conversation that is not the reader's says so", async () => {
  const { data } = fakeData();
  // An ID of the server's form that no conversation has.
  const id = "01890a5d-ac96-774b-bcce-b302099a8057";
  await loadConversation(data, id);

  renderFeature(
    data,
    <ConversationView id={id} onBack={() => undefined} onHidden={() => undefined} reply={() => null} />,
  );

  expect(
    await screen.findByText("These direct messages were not found. Check the link, or go back to the list."),
  ).not.toBeNull();
});
