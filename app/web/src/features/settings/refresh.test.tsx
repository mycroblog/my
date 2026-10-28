// SPDX-License-Identifier: AGPL-3.0-only
// The writes of the settings are in the lists that no page shows when they are made (web-client/README.md S1, ADR-0045):
// a timeline opened afterwards shows the change from its first render. The server answers every list slowly, as
// over a real network, so that a list shown from a stale cache would be seen.
import { screen, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import type { Preferences, PreferencesChange } from "./api/me";
import {
  otherAccountFixture,
  preferencesFixture,
  relationshipFixture,
  type Schemas,
  statusPage,
  timelineStatusFixture,
} from "../../test/fixtures";
import { apiBase, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

/**
 * How long the server takes to answer a timeline, in ms. The waits keep the setup's asyncUtilTimeout
 * (web-client/README.md T7): a loaded machine stretches this latency well beyond any multiple of it.
 */
const latency = 300;

const japanesePost: Schemas["Status"] = {
  ...timelineStatusFixture,
  id: "0192f0c4-7a8e-7000-8000-0000000000a2",
  content: "<p>こんにちは</p>",
  language: "ja",
};

function localFeed() {
  return screen.getByRole("feed", { name: "Local timeline" });
}

describe("a change of the timeline languages", () => {
  test("is on the local timeline at once when it opens again", async () => {
    let preferences: Preferences = { ...preferencesFixture, timeline_languages: [] };
    server.use(
      resumableSession(),
      http.get(`${apiBase}/preferences`, () => HttpResponse.json(preferences)),
      http.patch(`${apiBase}/preferences`, async ({ request }) => {
        const change = (await request.json()) as PreferencesChange;
        preferences = { ...preferences, ...change };
        return HttpResponse.json(preferences);
      }),
      http.get(`${apiBase}/timelines/local`, async () => {
        await delay(latency);
        const languages = preferences.timeline_languages;
        const posts = [timelineStatusFixture, japanesePost].filter(
          (post) => languages.length === 0 || languages.includes(post.language),
        );
        return HttpResponse.json(statusPage(posts));
      }),
    );
    const { user, router } = await renderApp({ path: "/local" });
    expect(
      await within(await screen.findByRole("feed", { name: "Local timeline" })).findByRole("article", {
        description: "Hello from the local timeline!",
      }),
    ).toBeInTheDocument();

    await router.navigate({ to: "/settings/preferences" });
    const languages = await screen.findByRole("group", { name: "Languages of your timelines" });
    await user.click(within(languages).getByRole("checkbox", { name: "Japanese" }));
    // The change is saved once the timelines it filters have been fetched again.
    expect(await screen.findByText("Saved", {})).toBeInTheDocument();

    await router.navigate({ to: "/local" });
    expect(await screen.findByRole("heading", { level: 1, name: "Local timeline" })).toBeInTheDocument();
    // The timeline shows only the posts in Japanese from its first render, not the list from before the change.
    expect(within(localFeed()).getByRole("article", { description: "こんにちは" })).toBeInTheDocument();
    expect(
      within(localFeed()).queryByRole("article", { description: "Hello from the local timeline!" }),
    ).not.toBeInTheDocument();
  });
});

describe("an unblock from the settings", () => {
  test("is on the local timeline at once when it opens again", async () => {
    let blocked = true;
    server.use(
      resumableSession(),
      http.get(`${apiBase}/blocks`, () => HttpResponse.json({ accounts: blocked ? [otherAccountFixture] : [] })),
      http.post(`${apiBase}/accounts/:id/unblock`, ({ params }) => {
        blocked = false;
        return HttpResponse.json(relationshipFixture(String(params["id"])));
      }),
      http.get(`${apiBase}/timelines/local`, async () => {
        await delay(latency);
        return HttpResponse.json(statusPage(blocked ? [] : [timelineStatusFixture]));
      }),
    );
    const { user, router } = await renderApp({ path: "/local" });
    expect(await screen.findByRole("heading", { level: 2, name: "No posts yet" })).toBeInTheDocument();

    await router.navigate({ to: "/settings/blocks" });
    await user.click(await screen.findByRole("button", { name: "Unblock: Bob" }));
    // The unblock is done once the lists it changes have been fetched again.
    expect(await screen.findByText("You unblocked Bob.", {})).toBeInTheDocument();

    await router.navigate({ to: "/local" });
    expect(await screen.findByRole("heading", { level: 1, name: "Local timeline" })).toBeInTheDocument();
    // The timeline shows Bob's post from its first render, not the empty list from before the unblock.
    expect(
      within(localFeed()).getByRole("article", { description: "Hello from the local timeline!" }),
    ).toBeInTheDocument();
  });
});
