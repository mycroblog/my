// SPDX-License-Identifier: AGPL-3.0-only
// The first run of a new user through the whole app (web-client/README.md T4, web-client/screens/welcome.md and
// home-first-run.md): the welcome step saves the basics of the profile and opens the home timeline, whose checklist
// counts the steps done and whose suggestions offer accounts to follow (FR-308).
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { accountFixture, instanceFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, resumableSession, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { suggestionsHandler } from "./test/handlers";

const bob = accountFixture(21, "Bob", instanceFixture.domain);
const carol = accountFixture(22, "Carol", instanceFixture.domain);

/** A new viewer: no display name, no posts, nobody followed. Following someone or saving the profile changes it. */
function newViewerServer() {
  let viewer: Schemas["CredentialAccount"] = {
    ...viewerFixture,
    display_name: "",
    statuses_count: 0,
    followers_count: 0,
    following_count: 0,
  };
  const changes: Schemas["UpdateCredentialsRequest"][] = [];
  server.use(
    resumableSession(),
    http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) =>
      withViewer(request, () => HttpResponse.json(viewer)),
    ),
    http.patch(`${apiBase}/accounts/update_credentials`, async ({ request }) => {
      const change = (await request.json()) as Schemas["UpdateCredentialsRequest"];
      changes.push(change);
      viewer = { ...viewer, display_name: change.display_name ?? viewer.display_name };
      return HttpResponse.json(viewer);
    }),
    http.post(`${apiBase}/accounts/:id/follow`, ({ params }) => {
      viewer = { ...viewer, following_count: 1 };
      return HttpResponse.json({
        id: String(params["id"]),
        following: true,
        followed_by: false,
        requested: false,
        blocking: false,
        blocked_by: false,
        muting: false,
        muting_notifications: false,
        endorsed: false,
        note: "",
      });
    }),
    suggestionsHandler([
      { source: "featured", account: bob },
      { source: "recently_active", account: carol },
    ]),
  );
  return { changes };
}

describe("the welcome step", () => {
  test("saves the display name and opens the home timeline", async () => {
    const { changes } = newViewerServer();
    const { user, router } = await renderApp({ path: "/welcome" });

    expect(await screen.findByRole("heading", { level: 1, name: /Welcome to/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Profile basics" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Display name" }), "Alice A.");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/home");
    });
    expect(changes).toEqual([{ display_name: "Alice A." }]);
  });

  test("goes on without saving when nothing changed, and Later skips it", async () => {
    const { changes } = newViewerServer();
    const { user, router } = await renderApp({ path: "/welcome" });

    expect(await screen.findByRole("link", { name: "Later" })).toHaveAttribute("href", "/home");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/home");
    });
    expect(changes).toEqual([]);
  });
});

describe("the first run on the home timeline", () => {
  test("counts the steps, suggests accounts, and marks the follow step done after a follow", async () => {
    newViewerServer();
    const { user } = await renderApp({ path: "/home" });

    const checklist = await screen.findByRole("region", { name: "Get started" });
    expect(within(checklist).getByRole("progressbar", { name: "Steps done" })).toHaveAttribute(
      "aria-valuetext",
      "0 of 3 done",
    );
    const suggestions = await screen.findByRole("region", { name: "Suggested accounts" });
    expect(within(suggestions).getByText("Featured by the server")).toBeInTheDocument();
    expect(within(suggestions).getByText("Posted recently")).toBeInTheDocument();
    expect(within(suggestions).getByRole("link", { name: "Find more people" })).toHaveAttribute("href", "/search");

    const bobRow = within(suggestions).getAllByRole("listitem")[0];
    if (bobRow === undefined) throw new Error("no suggestion rows");
    await user.click(await within(bobRow).findByRole("button", { name: "Follow" }));

    await waitFor(() => {
      expect(within(checklist).getByRole("progressbar", { name: "Steps done" })).toHaveAttribute(
        "aria-valuetext",
        "1 of 3 done",
      );
    });
    // The suggestions stay until the page is left, so that the follow's result stays in sight.
    expect(await within(bobRow).findByRole("button", { name: "Following" })).toBeInTheDocument();
  });

  test("closes the checklist and moves the focus to the page heading", async () => {
    newViewerServer();
    const { user } = await renderApp({ path: "/home" });

    const checklist = await screen.findByRole("region", { name: "Get started" });
    await user.click(within(checklist).getByRole("button", { name: "Hide these steps" }));

    await waitFor(() => {
      expect(screen.queryByRole("region", { name: "Get started" })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("heading", { level: 1, name: "Home" })).toHaveFocus();
    // Nobody is followed yet: the suggestions stay.
    expect(screen.getByRole("region", { name: "Suggested accounts" })).toBeInTheDocument();
  });

  test("shows nothing to a user who has done every step", async () => {
    server.use(resumableSession());
    await renderApp({ path: "/home" });

    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Get started" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Suggested accounts" })).not.toBeInTheDocument();
  });
});
