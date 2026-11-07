// SPDX-License-Identifier: AGPL-3.0-only
// The automatic deletion of old posts through the whole app (web-client/README.md T4): the policy the server has, editing
// it and saving it as a whole, the count of what the edited policy deletes, a failed save, leaving with unsaved changes,
// and the server maximum (ADR-0151): what it allows, what it locks, and the notice when it binds the user, with MSW
// playing the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { sectionHeadings } from "../../test/sections";
import {
  autoDeleteLimitFixture,
  autoDeletePolicyFixture,
  autoDeletePreviewOf,
  autoDeleteSettingsUnder,
} from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** The server's policy, which a save replaces, with the bodies it received. */
function autoDeleteServer(initial: Schemas["AutoDeletePolicy"]) {
  let policy = initial;
  const saves: Schemas["AutoDeletePolicy"][] = [];
  server.use(
    http.get(`${apiBase}/preferences/auto_delete`, () => HttpResponse.json(policy)),
    http.put(`${apiBase}/preferences/auto_delete`, async ({ request }) => {
      const body = (await request.json()) as Schemas["AutoDeletePolicy"];
      saves.push(body);
      policy = body;
      return HttpResponse.json(policy);
    }),
  );
  return { saves };
}

describe("automatic deletion of old posts", () => {
  test("is in the settings menu", async () => {
    await renderApp({ path: "/settings" });
    const link = await screen.findByRole("link", { name: "Delete old posts" });
    expect(link).toHaveAttribute("href", "/settings/auto-delete");
  });

  test("the sections follow the screen's requirements, and the save button ends the form", async () => {
    autoDeleteServer(autoDeletePolicyFixture);
    await renderApp({ path: "/settings/auto-delete" });
    expect(await screen.findByRole("heading", { level: 1, name: "Delete old posts" })).toBeInTheDocument();
    // docs/architecture/web-client/screens/settings-auto-delete.md §3
    expect(sectionHeadings()).toEqual([
      "Automatic deletion",
      "Keep these posts",
      "Keep posts with these visibilities",
      "Posts with many reactions",
      "Posts these conditions delete",
    ]);
    expect(screen.getByRole("group", { name: "Keep these posts" })).toHaveAccessibleDescription(
      "Posts that match any checked item are never deleted automatically.",
    );
    // A form saved by its button has no switch (web-client/21-experience-principles.md §21.4).
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });

  test("shows the saved policy and saves the edited one as a whole", async () => {
    const api = autoDeleteServer(autoDeletePolicyFixture);
    const { user } = await renderApp({ path: "/settings/auto-delete" });

    expect(await screen.findByRole("heading", { level: 1, name: "Delete old posts" })).toBeInTheDocument();
    const enabled = screen.getByRole("checkbox", { name: /Delete old posts automatically/ });
    expect(enabled).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Pinned to your profile" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "With a poll" })).not.toBeChecked();
    expect(screen.getByRole("group", { name: "Keep posts with these visibilities" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Mentioned people" })).toBeChecked();

    await user.click(enabled);
    await user.click(screen.getByRole("button", { name: /Delete posts older than/ }));
    await user.click(await screen.findByRole("option", { name: "14 days" }));
    await user.click(screen.getByRole("checkbox", { name: "With images or other media" }));
    await user.click(screen.getByRole("checkbox", { name: "Followers" }));
    await user.click(screen.getByRole("checkbox", { name: "Bookmarked by you" }));
    await user.click(screen.getByRole("button", { name: /Keep posts boosted at least/ }));
    await user.click(await screen.findByRole("option", { name: "5 times" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(
      await screen.findByText("Saved. Posts older than 14 days will be deleted, except those you keep."),
    ).toBeInTheDocument();
    expect(api.saves).toEqual([
      {
        ...autoDeletePolicyFixture,
        enabled: true,
        min_age_days: 14,
        keep_bookmarked: false,
        keep_media: true,
        keep_visibilities: ["private", "direct"],
        keep_min_boosts: 5,
      },
    ]);
  });

  test("counts what the edited conditions delete before they are saved", async () => {
    const api = autoDeleteServer(autoDeletePolicyFixture);
    const previews: Schemas["AutoDeletePolicy"][] = [];
    server.use(
      http.post(`${apiBase}/preferences/auto_delete/preview`, async ({ request }) => {
        const body = (await request.json()) as Schemas["AutoDeletePolicy"];
        previews.push(body);
        return HttpResponse.json(autoDeletePreviewOf(body));
      }),
    );
    const { user } = await renderApp({ path: "/settings/auto-delete" });
    const section = await screen.findByRole("region", { name: "Posts these conditions delete" });
    const count = () => within(section).getByRole("status");
    // The default policy deletes the six old posts that pins, bookmarks, favourites and direct posts do not keep.
    await waitFor(() => {
      expect(count()).toHaveTextContent(/Posts to delete now\s*6$/);
    });
    expect(within(section).getAllByRole("group", { name: "Post to delete" })).toHaveLength(3);
    expect(within(section).getByRole("list", { name: "The oldest of them" })).toBeInTheDocument();

    await user.click(screen.getByRole("checkbox", { name: "With images or other media" }));
    await waitFor(() => {
      expect(count()).toHaveTextContent(/Posts to delete now\s*5$/);
    });
    await user.click(screen.getByRole("button", { name: /Keep posts boosted at least/ }));
    await user.click(await screen.findByRole("option", { name: "5 times" }));
    await waitFor(() => {
      expect(count()).toHaveTextContent(/Posts to delete now\s*4$/);
    });
    // Every condition of the draft goes to the server, turned on whether or not the form is, and nothing is saved.
    expect(previews.at(-1)).toEqual({
      ...autoDeletePolicyFixture,
      enabled: true,
      keep_media: true,
      keep_min_boosts: 5,
    });
    expect(api.saves).toEqual([]);

    // Turning the policy on asks for no new count.
    const asked = previews.length;
    await user.click(screen.getByRole("checkbox", { name: /Delete old posts automatically/ }));
    expect(previews).toHaveLength(asked);
  });

  test("says when the posts could not be counted", async () => {
    autoDeleteServer(autoDeletePolicyFixture);
    server.use(
      http.post(`${apiBase}/preferences/auto_delete/preview`, () =>
        problem(500, "about:blank", "Internal Server Error"),
      ),
    );
    await renderApp({ path: "/settings/auto-delete" });
    expect(await screen.findByText(/^The posts could not be counted: /)).toBeVisible();
  });

  test("offers a saved age that is not among the choices", async () => {
    autoDeleteServer({ ...autoDeletePolicyFixture, enabled: true, min_age_days: 45 });
    await renderApp({ path: "/settings/auto-delete" });
    expect(await screen.findByRole("button", { name: /45 days/ })).toBeInTheDocument();
  });

  test("keeps the edits when the save fails", async () => {
    server.use(
      http.put(`${apiBase}/preferences/auto_delete`, () =>
        problem(422, "about:blank", "The request cannot be processed."),
      ),
    );
    const { user } = await renderApp({ path: "/settings/auto-delete" });
    await user.click(await screen.findByRole("checkbox", { name: /Delete old posts automatically/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText(/^The settings could not be saved: /)).toBeVisible();
    expect(
      screen.getAllByRole("alert").filter((alert) => alert.textContent.startsWith("The settings could not be saved: ")),
    ).toHaveLength(1);
    expect(screen.getByRole("checkbox", { name: /Delete old posts automatically/ })).toBeChecked();
  });

  test("asks before leaving with unsaved changes", async () => {
    const { user } = await renderApp({ path: "/settings/auto-delete" });
    await user.click(await screen.findByRole("checkbox", { name: "With a poll" }));
    await user.click(screen.getByRole("link", { name: "Settings" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Leave without saving?" });
    expect(dialog).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Keep editing" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("checkbox", { name: "With a poll" })).toBeChecked();
  });
});

/**
 * The server's policy under the maximum of autoDeleteLimitFixture: GET answers it with what the maximum makes of it,
 * and the preview counts under the maximum. A save over the maximum is refused as the server does, naming the property.
 */
function limitedServer(initial: Schemas["AutoDeletePolicy"], enforceFrom?: string) {
  let policy = initial;
  const saves: Schemas["AutoDeletePolicy"][] = [];
  const previews: Schemas["AutoDeletePolicy"][] = [];
  server.use(
    http.get(`${apiBase}/preferences/auto_delete`, () =>
      HttpResponse.json(autoDeleteSettingsUnder(policy, autoDeleteLimitFixture, enforceFrom)),
    ),
    http.put(`${apiBase}/preferences/auto_delete`, async ({ request }) => {
      const body = (await request.json()) as Schemas["AutoDeletePolicy"];
      saves.push(body);
      policy = body;
      return HttpResponse.json(autoDeleteSettingsUnder(policy, autoDeleteLimitFixture));
    }),
    http.post(`${apiBase}/preferences/auto_delete/preview`, async ({ request }) => {
      const body = (await request.json()) as Schemas["AutoDeletePolicy"];
      previews.push(body);
      return HttpResponse.json(autoDeletePreviewOf(body, autoDeleteLimitFixture));
    }),
  );
  return { saves, previews };
}

/** A policy within the maximum of autoDeleteLimitFixture: on, 30 days, keeping pins and bookmarks. */
const withinLimit: Schemas["AutoDeletePolicy"] = {
  ...autoDeletePolicyFixture,
  enabled: true,
  keep_favourited: false,
  keep_visibilities: [],
};

describe("automatic deletion under the server's maximum", () => {
  test("explains the maximum, keeps the deletion on and locks what it does not allow", async () => {
    limitedServer(withinLimit);
    const { user } = await renderApp({ path: "/settings/auto-delete" });
    const limit = await screen.findByRole("region", { name: "This server's limit" });
    expect(sectionHeadings()[0]).toBe("This server's limit");
    expect(limit).toHaveTextContent("posts are deleted at most 60 days after they're published");
    const allowed = within(limit).getByRole("list", { name: "What this server lets you keep" });
    expect(
      within(allowed)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Pinned to your profile, the 3 most recently pinned", "Bookmarked by you"]);
    // Not bound: no notice of a date.
    expect(within(limit).queryByText(/this server's limit decides/)).not.toBeInTheDocument();

    const enabled = screen.getByRole("checkbox", { name: /Delete old posts automatically/ });
    expect(enabled).toBeChecked();
    expect(enabled).toBeDisabled();
    const media = screen.getByRole("checkbox", { name: "With images or other media" });
    expect(media).toBeDisabled();
    expect(media).toHaveAccessibleDescription("This server's limit doesn't allow this.");
    expect(screen.getByRole("checkbox", { name: "Pinned to your profile" })).toHaveAccessibleDescription(
      "This server keeps only your 3 most recently pinned posts.",
    );
    expect(screen.getByRole("checkbox", { name: "Bookmarked by you" })).toBeEnabled();
    expect(screen.getByRole("checkbox", { name: "Public" })).toBeDisabled();
    expect(screen.getByRole("group", { name: "Keep posts with these visibilities" })).toHaveAccessibleDescription(
      /This server's limit doesn't allow this\.$/,
    );
    expect(screen.getByRole("button", { name: /Keep posts boosted at least/ })).toBeDisabled();

    // The ages stop at the maximum.
    await user.click(screen.getByRole("button", { name: /Delete posts older than/ }));
    const ages = (await screen.findAllByRole("option")).map((option) => option.textContent);
    expect(ages).toEqual(["7 days", "14 days", "30 days", "60 days"]);
  });

  test("tells a bound user from when the maximum applies, how, and what goes then", async () => {
    // The default policy is off and keeps favourites and direct posts, which the maximum does not allow.
    const api = limitedServer(autoDeletePolicyFixture, "2026-12-01T00:00:00Z");
    const { user } = await renderApp({ path: "/settings/auto-delete" });
    const limit = await screen.findByRole("region", { name: "This server's limit" });
    expect(
      within(limit).getByText(/^From .*, this server's limit decides which of your posts are deleted\.$/),
    ).toBeVisible();
    expect(within(limit).getByText("December 1, 2026").tagName).toBe("TIME");
    expect(
      within(within(limit).getByRole("list", { name: /^Until then, your settings below apply/ }))
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Pinned to your profile, the 3 most recently pinned", "Bookmarked by you"]);
    // Posts older than 60 days that neither a pin nor a bookmark keeps: four of the fake posts.
    await waitFor(() => {
      expect(within(limit).getByRole("status")).toHaveTextContent(/^Posts that go then\s*4$/);
    });

    // The saved values beyond the maximum stay, marked; the save sends nothing until they are changed.
    const enabled = screen.getByRole("checkbox", { name: /Delete old posts automatically/ });
    expect(enabled).not.toBeChecked();
    expect(enabled).toBeEnabled();
    expect(enabled).toBeInvalid();
    expect(screen.getByRole("checkbox", { name: "Favourited by you" })).toBeInvalid();
    expect(screen.getByRole("checkbox", { name: "Mentioned people" })).toBeInvalid();
    await user.click(screen.getByRole("button", { name: "Save" }));
    // The browser keeps the form from being sent and moves the focus to the first marked field.
    await waitFor(() => {
      expect(enabled).toHaveFocus();
    });
    expect(enabled).toHaveAccessibleDescription(/Turn it on to save\.$/);
    expect(api.saves).toEqual([]);

    await user.click(enabled);
    await user.click(screen.getByRole("checkbox", { name: "Favourited by you" }));
    await user.click(screen.getByRole("checkbox", { name: "Mentioned people" }));
    // Once within the maximum, nothing is locked by the values beyond it any more.
    expect(screen.getByRole("checkbox", { name: "Favourited by you" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(
      await screen.findByText("Saved. Posts older than 30 days will be deleted, except those you keep."),
    ).toBeInTheDocument();
    expect(api.saves).toEqual([withinLimit]);
    // Saved within the maximum, the user is no longer bound.
    expect(screen.queryByText(/this server's limit decides/)).not.toBeInTheDocument();
  });

  test("says the maximum applies soon before the user was told the date", async () => {
    limitedServer(autoDeletePolicyFixture);
    await renderApp({ path: "/settings/auto-delete" });
    expect(
      await screen.findByText("Soon, this server's limit will decide which of your posts are deleted."),
    ).toBeVisible();
  });

  test("marks an age beyond the maximum, and offers it until it is changed", async () => {
    limitedServer({ ...withinLimit, min_age_days: 90 });
    await renderApp({ path: "/settings/auto-delete" });
    const age = await screen.findByRole("button", { name: /Delete posts older than/ });
    expect(age).toHaveTextContent("90 days");
    expect(
      await screen.findByText("This server keeps posts at most 60 days. Choose a shorter age to save."),
    ).toBeVisible();
  });

  test("shows the property a refused save names on its field", async () => {
    limitedServer(withinLimit);
    server.use(
      http.put(`${apiBase}/preferences/auto_delete`, () =>
        HttpResponse.json(
          {
            type: "about:blank",
            title: "The request cannot be processed.",
            status: 422,
            detail: "min_age_days is not valid.",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/settings/auto-delete" });
    await user.click(await screen.findByRole("checkbox", { name: "Bookmarked by you" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    // Beside the save button with the reason, and under the field it names.
    expect(await screen.findByText("The settings could not be saved: min_age_days is not valid.")).toBeVisible();
    expect(screen.getByText("min_age_days is not valid.")).toBeVisible();
  });
});
