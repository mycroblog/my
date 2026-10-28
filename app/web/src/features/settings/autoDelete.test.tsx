// SPDX-License-Identifier: AGPL-3.0-only
// The automatic deletion of old posts through the whole app (web-client/README.md T4): the policy the server has, editing
// it and saving it as a whole, the count of what the edited policy deletes, a failed save, and leaving with unsaved changes, with MSW playing the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { sectionHeadings } from "../../test/sections";
import { autoDeletePolicyFixture, autoDeletePreviewOf } from "./test/handlers";

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
