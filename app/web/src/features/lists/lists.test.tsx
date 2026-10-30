// SPDX-License-Identifier: AGPL-3.0-only
// The lists through the whole app (web-client/README.md T4; web-client/screens/lists.md, list-timeline.md, profile.md):
// the real router and QueryClient, with MSW as the server. A list is created, gets a followed account, shows its
// timeline with the stream's new entries, is renamed and deleted; "Add to list" is offered only for a followed account.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import {
  otherAccountFixture,
  relationshipFixture,
  remoteAccountFixture,
  type Schemas,
  timelineStatusFixture,
} from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { newPostsHeld, streamingServer } from "../../test/msw/streaming";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { friendsList, listFixture } from "./test/handlers";

const fsi = (text: string) => `⁨${text}⁩`;

/** A server whose lists change with the requests, starting from `initial`. */
function listsServer(initial: Schemas["List"][]) {
  let lists = [...initial];
  const members = new Map<string, Schemas["Account"][]>();
  server.use(
    http.get(`${apiBase}/lists`, () => HttpResponse.json({ items: lists })),
    http.post(`${apiBase}/lists`, async ({ request }) => {
      const { name } = (await request.json()) as Schemas["ListRequest"];
      const list = listFixture(`0192f0c4-7a8e-7000-8000-00000000a1${String(lists.length).padStart(2, "0")}`, name);
      lists = [...lists, list];
      return HttpResponse.json(list);
    }),
    http.get(`${apiBase}/lists/:id`, ({ params }) => {
      const list = lists.find((item) => item.id === params["id"]);
      return list === undefined ? problem(404, "/api/v1/problems/not-found", "Not Found") : HttpResponse.json(list);
    }),
    http.patch(`${apiBase}/lists/:id`, async ({ request, params }) => {
      const { name } = (await request.json()) as Schemas["ListRequest"];
      lists = lists.map((item) => (item.id === params["id"] ? { ...item, name } : item));
      return HttpResponse.json(lists.find((item) => item.id === params["id"]));
    }),
    http.delete(`${apiBase}/lists/:id`, ({ params }) => {
      lists = lists.filter((item) => item.id !== params["id"]);
      return new HttpResponse(null, { status: 204 });
    }),
    http.get(`${apiBase}/lists/:id/accounts`, ({ params }) =>
      HttpResponse.json({ accounts: members.get(String(params["id"])) ?? [] }),
    ),
    http.post(`${apiBase}/lists/:id/accounts/:accountId`, ({ params }) => {
      const id = String(params["id"]);
      const account = [otherAccountFixture, remoteAccountFixture].find((a) => a.id === params["accountId"]);
      if (account !== undefined) members.set(id, [...(members.get(id) ?? []), account]);
      lists = lists.map((item) => (item.id === id ? { ...item, members_count: item.members_count + 1 } : item));
      return new HttpResponse(null, { status: 204 });
    }),
    http.delete(`${apiBase}/lists/:id/accounts/:accountId`, ({ params }) => {
      const id = String(params["id"]);
      members.set(
        id,
        (members.get(id) ?? []).filter((a) => a.id !== params["accountId"]),
      );
      lists = lists.map((item) => (item.id === id ? { ...item, members_count: item.members_count - 1 } : item));
      return new HttpResponse(null, { status: 204 });
    }),
    http.get(`${apiBase}/accounts/:id/lists`, ({ params }) =>
      HttpResponse.json({
        items: lists.filter((list) => (members.get(list.id) ?? []).some((a) => a.id === params["id"])),
      }),
    ),
    // The search finds Bob, whom the viewer follows, and Carol, whom the viewer does not.
    http.get(`${apiBase}/accounts/search`, () => HttpResponse.json([otherAccountFixture, remoteAccountFixture])),
    http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
      HttpResponse.json(
        new URL(request.url).searchParams
          .getAll("id")
          .map((id) => relationshipFixture(id, { following: id === otherAccountFixture.id })),
      ),
    ),
  );
}

describe("the lists page", () => {
  test("creates a list, adds a followed account to it, renames it and deletes it", async () => {
    server.use(resumableSession());
    listsServer([]);
    const { user } = await renderApp({ path: "/lists" });
    expect(await screen.findByRole("heading", { level: 1, name: "Lists" })).toBeInTheDocument();

    // Create: the button, the name, Create.
    await user.click(await screen.findByRole("button", { name: "Create a list" }));
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Friends");
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findByText(`Created ${fsi("Friends")}.`)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Friends" })).toHaveAttribute("href", expect.stringMatching(/^\/lists\//));
    // Focus goes back to the button that opened the form.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Create a list" })).toHaveFocus();
    });

    // Add a member: Manage members, the search, Add. Only the followed account can be added.
    const manage = screen.getByRole("button", { name: `Manage members: ${fsi("Friends")}` });
    await user.click(manage);
    expect(manage).toHaveAttribute("aria-expanded", "true");
    const members = await screen.findByRole("region", { name: `Members of ${fsi("Friends")}` });
    await user.type(within(members).getByRole("searchbox", { name: "Search the people you follow" }), "b");
    const results = await within(members).findByRole("list", { name: "People you follow" });
    expect(within(results).getAllByRole("listitem")).toHaveLength(1);
    await user.click(await within(results).findByRole("button", { name: `Add ${fsi("Bob")}` }));
    expect(await within(results).findByText("Added")).toBeInTheDocument();
    const memberList = await within(members).findByRole("list", { name: "Members" });
    expect(within(memberList).getByRole("link", { name: /Bob/ })).toBeInTheDocument();
    expect(await screen.findByText("1 member")).toBeInTheDocument();

    // Rename: Rename, the name, Save; focus goes back to Rename.
    await user.click(screen.getByRole("button", { name: `Rename: ${fsi("Friends")}` }));
    const field = await screen.findByRole("textbox", { name: "Name" });
    await user.clear(field);
    await user.type(field, "Close friends");
    await user.click(screen.getByRole("button", { name: "Save" }));
    const rename = await screen.findByRole("button", { name: `Rename: ${fsi("Close friends")}` });
    await waitFor(() => {
      expect(rename).toHaveFocus();
    });

    // Delete: Delete, then Delete in the dialog.
    await user.click(screen.getByRole("button", { name: `Delete: ${fsi("Close friends")}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Delete ${fsi("Close friends")}?` });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await screen.findByRole("heading", { name: "No lists yet" })).toBeInTheDocument();
  });

  test("says why no list can be created once there are 50", async () => {
    server.use(resumableSession());
    listsServer(
      Array.from({ length: 50 }, (_, i) =>
        listFixture(`0192f0c4-7a8e-7000-8000-00000000b${String(i).padStart(3, "0")}`, `List ${String(i + 1)}`),
      ),
    );
    await renderApp({ path: "/lists" });
    expect(
      await screen.findByText("You can have up to 50 lists. Delete a list you no longer use to create another."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create a list" })).not.toBeInTheDocument();
  });
});

describe("a list's timeline", () => {
  test("shows the members' posts under the list's name, and the stream's new entries behind the button", async () => {
    const streams = streamingServer();
    // The user turned the automatic insertion off, so the new entries wait behind the button.
    server.use(resumableSession(), streams.handler, newPostsHeld());
    const { user } = await renderApp({ path: `/lists/${friendsList.id}` });
    expect(await screen.findByRole("heading", { level: 1, name: "Friends" })).toBeInTheDocument();
    expect(screen.getByText("1 member")).toBeInTheDocument();
    const feed = await screen.findByRole("feed", { name: "Friends" });
    expect(within(feed).getAllByRole("article")).toHaveLength(1);
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    const id = "0192f0c4-7a8e-7000-8000-000000000301";
    const status = { ...timelineStatusFixture, id, content: "<p>New in the list</p>" };
    // An entry of another list is not this one's.
    streams.current().send("list_update", { list_id: "0192f0c4-7a8e-7000-8000-00000000ffff", status });
    streams.current().send("list_update", { list_id: friendsList.id, status });
    await user.click(await screen.findByRole("button", { name: "1 new post" }));
    const articles = await within(feed).findAllByRole("article");
    expect(articles[0]).toHaveAccessibleDescription("New in the list");
    expect(screen.getByRole("heading", { level: 1, name: "Friends" })).toHaveFocus();
  });

  test("leads a list without members to managing them", async () => {
    server.use(resumableSession());
    listsServer([listFixture(friendsList.id, "Empty")]);
    await renderApp({ path: `/lists/${friendsList.id}` });
    expect(await screen.findByRole("heading", { name: "No one is in this list yet" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage members" })).toHaveAttribute("href", "/lists");
    expect(screen.queryByRole("feed")).not.toBeInTheDocument();
  });

  test("says that a list that is not the viewer's is not found", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/timelines/list/:id`, () => problem(404, "/api/v1/problems/not-found", "Not Found")),
    );
    await renderApp({ path: "/lists/0192f0c4-7a8e-7000-8000-00000000ffff" });
    expect(await screen.findByRole("heading", { level: 1, name: "List not found" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to your lists" })).toHaveAttribute("href", "/lists");
  });
});

describe("Add to list on a profile", () => {
  test("is offered for a followed account, and checking a list adds the account at once", async () => {
    server.use(resumableSession());
    listsServer([friendsList, listFixture("0192f0c4-7a8e-7000-8000-00000000a002", "Bakers")]);
    const { user } = await renderApp({ path: "/@bob" });
    await user.click(await screen.findByRole("button", { name: `More actions for ${fsi("Bob")}` }));
    await user.click(await screen.findByRole("menuitem", { name: "Add to list" }));
    const dialog = await screen.findByRole("dialog", { name: `Add ${fsi("Bob")} to lists` });
    const bakers = await within(dialog).findByRole("checkbox", { name: "Bakers" });
    expect(bakers).not.toBeChecked();
    await user.click(bakers);
    await waitFor(() => {
      expect(within(dialog).getByRole("checkbox", { name: "Bakers" })).toBeChecked();
    });
    expect(within(dialog).getByRole("link", { name: "Manage lists" })).toHaveAttribute("href", "/lists");
  });

  test("is not offered for an account the viewer does not follow", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
        HttpResponse.json(new URL(request.url).searchParams.getAll("id").map((id) => relationshipFixture(id))),
      ),
    );
    const { user } = await renderApp({ path: "/@bob" });
    await user.click(await screen.findByRole("button", { name: `More actions for ${fsi("Bob")}` }));
    await screen.findByRole("menuitem", { name: "Mute" });
    expect(screen.queryByRole("menuitem", { name: "Add to list" })).not.toBeInTheDocument();
  });
});
