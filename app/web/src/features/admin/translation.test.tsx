// SPDX-License-Identifier: AGPL-3.0-only
// The translation screen through the whole app (web-client/screens/admin-translation.md): an administrator with
// manage_settings sees the month's use and the warning about the privacy policy, enables and orders providers, sets what
// each may receive and the limits; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { parseProviderVisibilities, visibilitiesOf, withVisibilities } from "./api/translation";
import { instanceSettingsFixture, translationOverviewFixture } from "./test/handlers";

const path = "/admin/translation";

/** Answers the settings, with `values` in place of the defaults, and records what a save sends. */
function settings(values: Partial<Record<Schemas["InstanceSettingKey"], string>> = {}) {
  const sent: Record<string, string>[] = [];
  server.use(
    http.get(`${apiBase}/admin/settings`, () => HttpResponse.json(instanceSettingsFixture(values))),
    http.patch(`${apiBase}/admin/settings`, async ({ request }) => {
      const body = (await request.clone().json()) as { values: Record<string, string> };
      sent.push(body.values);
      Object.assign(values, body.values);
      return HttpResponse.json(instanceSettingsFixture(values));
    }),
  );
  return sent;
}

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler, operatorHandler(["manage_settings"]));
});

describe("the value of translation_provider_visibilities", () => {
  test("reads a provider not named as the default", () => {
    expect(visibilitiesOf("", "one")).toEqual(["public", "local", "unlisted"]);
    expect(visibilitiesOf("one=,two=public|direct", "one")).toEqual([]);
    expect(visibilitiesOf("one=,two=direct|public", "two")).toEqual(["public", "direct"]);
  });

  test("writes the form the server stores, and keeps the other pairs", () => {
    expect(withVisibilities("", "two", ["direct", "public"])).toBe("two=public|direct");
    expect(withVisibilities("gone=public,two=public", "two", [])).toBe("gone=public,two=");
    expect(withVisibilities("gone=public,two=public", "two", ["unlisted", "local", "public"])).toBe("gone=public");
    expect([...parseProviderVisibilities("").keys()]).toEqual([]);
  });
});

describe("the translation screen", () => {
  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_webhooks"]));
    await renderApp({ path });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("shows the month's characters and the cache hit rate", async () => {
    settings();
    server.use(
      http.get(`${apiBase}/admin/translation`, () =>
        HttpResponse.json(
          translationOverviewFixture({ characters: 12_345, cache_hits: 3, cache_misses: 1, cache_hit_rate: 0.75 }),
        ),
      ),
    );
    await renderApp({ path });

    expect(await screen.findByText("12,345")).toBeInTheDocument();
    expect(screen.getByText("Characters sent (November 2026)")).toBeInTheDocument();
    expect(screen.getByText("75%")).toBeInTheDocument();
  });

  test("says that there is no rate before any request", async () => {
    settings();
    await renderApp({ path });

    expect(await screen.findByText("No requests yet")).toBeInTheDocument();
  });

  test("explains the privacy policy before a provider is enabled", async () => {
    settings();
    await renderApp({ path });

    expect(
      await screen.findByText(/Update your privacy policy before you enable a server-side provider/u),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open the terms and policies" })).toHaveAttribute(
      "href",
      "/admin/documents",
    );
  });

  test("says when the server has no provider, and still offers the browser's translation", async () => {
    settings();
    server.use(
      http.get(`${apiBase}/admin/translation`, () => HttpResponse.json(translationOverviewFixture({ providers: [] }))),
    );
    await renderApp({ path });

    expect(
      await screen.findByRole("heading", { level: 3, name: "No server-side provider on this server" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: /built-in translation/u })).not.toBeChecked();
  });

  test("enables and orders providers, and sends only what changed", async () => {
    const user = userEvent.setup();
    const sent = settings({ translation_providers: "gone,one" });
    await renderApp({ path });

    await user.click(await screen.findByRole("checkbox", { name: "Provider Two" }));
    expect(screen.getByRole("button", { name: "Move down: Provider Two" })).toBeDisabled();
    const up = screen.getByRole("button", { name: "Move up: Provider Two" });
    await user.click(up);
    // Provider Two is first now: its "Move up" is disabled, and the focus is on its "Move down".
    expect(up).toBeDisabled();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Move down: Provider Two" })).toHaveFocus();
    });
    await user.click(screen.getByRole("button", { name: "Save the translation settings" }));

    await waitFor(() => {
      expect(sent).toEqual([{ translation_providers: "gone,two,one" }]);
    });
    expect(await screen.findByText("Saved the translation settings.")).toBeInTheDocument();
  });

  test("allows a provider posts for mentioned people, for that provider alone", async () => {
    const user = userEvent.setup();
    const sent = settings({ translation_providers: "one,two" });
    await renderApp({ path });

    const group = await screen.findByRole("group", { name: "What Provider One may receive" });
    expect(within(group).getByRole("checkbox", { name: "This server" })).toBeChecked();
    expect(within(group).getByRole("checkbox", { name: "Mentioned people" })).not.toBeChecked();
    await user.click(within(group).getByRole("checkbox", { name: "Mentioned people" }));
    await user.click(screen.getByRole("button", { name: "Save the translation settings" }));

    await waitFor(() => {
      expect(sent).toEqual([{ translation_provider_visibilities: "one=public|local|unlisted|direct" }]);
    });
  });

  test("lets reports go to the providers, which starts off", async () => {
    const user = userEvent.setup();
    const sent = settings({ translation_providers: "one" });
    await renderApp({ path });

    const reports = await screen.findByRole("checkbox", { name: "Send reports to the providers" });
    expect(reports).not.toBeChecked();
    await user.click(reports);
    await user.click(screen.getByRole("button", { name: "Save the translation settings" }));

    await waitFor(() => {
      expect(sent).toEqual([{ translation_reports_to_server: "true" }]);
    });
  });

  test("takes 0 as a limit, and refuses what is not a whole number in range", async () => {
    const user = userEvent.setup();
    const sent = settings();
    await renderApp({ path });

    const limit = await screen.findByRole("textbox", { name: /Characters per user a month/u });
    await user.clear(limit);
    await user.type(limit, "-1");
    await user.click(screen.getByRole("button", { name: "Save the translation settings" }));
    expect(await screen.findByText("Enter a whole number from 0 to 1000000000000.")).toBeInTheDocument();
    expect(sent).toEqual([]);

    await user.clear(limit);
    await user.type(limit, "0");
    await user.click(screen.getByRole("button", { name: "Save the translation settings" }));
    await waitFor(() => {
      expect(sent).toEqual([{ translation_user_monthly_characters: "0" }]);
    });
  });

  test("keeps the drafts and says why when a save fails", async () => {
    const user = userEvent.setup();
    settings();
    server.use(
      http.patch(`${apiBase}/admin/settings`, () =>
        HttpResponse.json({ type: "about:blank", title: "Refused", status: 422 }, { status: 422 }),
      ),
    );
    await renderApp({ path });

    const browser = await screen.findByRole("checkbox", { name: /built-in translation/u });
    await user.click(browser);
    await user.click(screen.getByRole("button", { name: "Save the translation settings" }));

    expect(await screen.findByText(/Could not save the translation settings/u)).toBeInTheDocument();
    expect(browser).toBeChecked();
  });

  test("keeps the form when the month's use fails to load, and tries again", async () => {
    settings();
    server.use(http.get(`${apiBase}/admin/translation`, () => new HttpResponse(null, { status: 500 })));
    await renderApp({ path });

    expect(
      await screen.findByRole("heading", { level: 3, name: "Could not load this month's use" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /Characters per user a month/u })).toBeInTheDocument();
  });
});
