// SPDX-License-Identifier: AGPL-3.0-only
// What a post's menu offers about its author, and the links of a moderator, through the whole app (web-client/README.md
// T4, FR-301 to FR-303): the real router and QueryClient, MSW for the client API.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import {
  instanceFixture,
  otherAccountFixture,
  relationshipFixture,
  remoteAccountFixture,
  remoteStatusFixture,
  type Schemas,
  statusFixture,
  statusPage,
  viewerFixture,
  viewerStatusFixture,
} from "../../test/fixtures";
import { apiBase, operatorHandler, problem, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const bobPost: Status = statusFixture("0192f0c4-7a8e-7000-8000-0000000000a1");
const secondBobPost: Status = statusFixture("0192f0c4-7a8e-7000-8000-0000000000a2");
const ownPost: Status = viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000a3" });
const bobPath = `/@bob/status/${bobPost.id}`;
const isolated = (handle: string) => `⁨${handle}⁩`;
const bob = isolated("@bob");

/** Serves `statuses` by ID. */
function serve(...statuses: Status[]) {
  server.use(
    http.get(`${apiBase}/statuses/:id`, ({ params }) => {
      const found = statuses.find((status) => status.id === params["id"]);
      return found === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(found);
    }),
  );
}

/** Answers the relationships with Bob and Carol as `overrides`. */
function serveRelationship(overrides: Partial<Schemas["Relationship"]>) {
  server.use(
    http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
      HttpResponse.json({
        items: new URL(request.url).searchParams.getAll("id").map((id) => relationshipFixture(id, overrides)),
      }),
    ),
  );
}

/** The requests for relationships the app sent, as their accounts. */
function recordRelationshipRequests() {
  const requests: string[][] = [];
  server.events.on("request:start", ({ request }) => {
    const url = new URL(request.url);
    if (url.pathname === `${apiBase}/accounts/relationships`) requests.push(url.searchParams.getAll("id"));
  });
  return requests;
}

/** The changes of a relationship the app sent, as their actions. */
function recordChanges(answer: Partial<Schemas["Relationship"]>) {
  const changes: string[] = [];
  server.use(
    http.post(`${apiBase}/accounts/:id/:action`, ({ params }) => {
      changes.push(`${String(params["id"])} ${String(params["action"])}`);
      return HttpResponse.json(relationshipFixture(String(params["id"]), answer));
    }),
    http.delete(`${apiBase}/accounts/:id/:relation`, ({ params }) => {
      changes.push(`${String(params["id"])} un${String(params["relation"])}`);
      return HttpResponse.json(relationshipFixture(String(params["id"]), answer));
    }),
  );
  return changes;
}

async function openMenu(user: Awaited<ReturnType<typeof renderApp>>["user"], index = 0) {
  const buttons = await screen.findAllByRole("button", { name: "More actions" });
  await user.click(buttons[index] as HTMLElement);
  return screen.findByRole("menu", { name: "More actions" });
}

async function findToast(text: string) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(resumableSession());
});

describe("reading the relationship", () => {
  test("a timeline reads none until a menu opens, then only that author's", async () => {
    const requests = recordRelationshipRequests();
    server.use(http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage([bobPost, secondBobPost]))));
    const { user } = await renderApp({ path: "/home" });

    await screen.findAllByRole("button", { name: "More actions" });
    expect(requests).toEqual([]);

    await openMenu(user);
    await waitFor(() => {
      expect(screen.getByRole("menuitem", { name: `Follow ${bob}` })).not.toHaveAttribute("aria-disabled");
    });
    expect(requests).toEqual([[otherAccountFixture.id]]);
  });

  test("until it is known, the items that depend on it stay in place, disabled, and the direct message works", async () => {
    serve(bobPost);
    server.use(
      http.get(`${apiBase}/accounts/relationships`, async () => {
        await delay("infinite");
        return HttpResponse.json({ items: [] });
      }),
    );
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);

    for (const name of [`Follow ${bob}`, `Mute ${bob}`, `Block ${bob}`]) {
      expect(await screen.findByRole("menuitem", { name })).toHaveAttribute("aria-disabled", "true");
    }
    expect(screen.getByRole("menuitem", { name: `Direct message ${bob}` })).not.toHaveAttribute("aria-disabled");
  });

  test("a failure takes their place with one item that reads it again", async () => {
    serve(bobPost);
    let isDown = true;
    server.use(
      http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
        isDown
          ? problem(503, "about:blank", "Service Unavailable")
          : HttpResponse.json({
              items: new URL(request.url).searchParams
                .getAll("id")
                .map((id) => relationshipFixture(id, { muting: true })),
            }),
      ),
    );
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);
    const retry = await screen.findByRole("menuitem", { name: /^Could not load whether you follow/ });
    expect(retry).toHaveTextContent("Service Unavailable");
    expect(screen.queryByRole("menuitem", { name: `Mute ${bob}` })).not.toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: `Direct message ${bob}` })).toBeInTheDocument();

    isDown = false;
    await user.click(retry);
    await openMenu(user);

    expect(await screen.findByRole("menuitem", { name: `Unmute ${bob}` })).toBeInTheDocument();
  });

  test("an account that the server leaves out offers nothing about its author", async () => {
    serve(bobPost);
    server.use(http.get(`${apiBase}/accounts/relationships`, () => HttpResponse.json({ items: [] })));
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);

    await waitFor(() => {
      expect(screen.queryByRole("menuitem", { name: `Mute ${bob}` })).not.toBeInTheDocument();
    });
    expect(screen.queryByRole("menuitem", { name: `Direct message ${bob}` })).not.toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Copy link" })).toBeInTheDocument();
  });
});

describe("following", () => {
  test("follows at once and says so in a toast", async () => {
    serve(bobPost);
    serveRelationship({});
    const changes = recordChanges({ following: true });
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);
    await waitFor(() => {
      expect(screen.getByRole("menuitem", { name: `Follow ${bob}` })).not.toHaveAttribute("aria-disabled");
    });
    await user.click(screen.getByRole("menuitem", { name: `Follow ${bob}` }));

    expect(await findToast(`You follow ${isolated("Bob")} now.`)).toBeInTheDocument();
    expect(changes).toEqual([`${otherAccountFixture.id} follow`]);

    await openMenu(user);
    expect(await screen.findByRole("menuitem", { name: `Unfollow ${bob}` })).toBeInTheDocument();
  });

  test("a remote author: the request stays pending, and is withdrawn after asking", async () => {
    const carol = isolated("@carol");
    serve(remoteStatusFixture);
    serveRelationship({});
    const changes = recordChanges({ requested: true });
    const { user } = await renderApp({ path: `/@carol@remote.example/status/${remoteStatusFixture.id}` });

    await openMenu(user);
    await waitFor(() => {
      expect(screen.getByRole("menuitem", { name: `Follow ${carol}` })).not.toHaveAttribute("aria-disabled");
    });
    await user.click(screen.getByRole("menuitem", { name: `Follow ${carol}` }));

    expect(await findToast(`Follow request sent to ${isolated("Carol")}.`)).toBeInTheDocument();
    expect(changes).toEqual([`${remoteAccountFixture.id} follow`]);

    await openMenu(user);
    await user.click(await screen.findByRole("menuitem", { name: `Cancel follow request to ${carol}` }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Withdraw request" }));

    await waitFor(() => {
      expect(changes).toEqual([`${remoteAccountFixture.id} follow`, `${remoteAccountFixture.id} unfollow`]);
    });
  });

  test("unfollowing asks first, as on a profile", async () => {
    serve(bobPost);
    serveRelationship({ following: true });
    const changes = recordChanges({});
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);
    await user.click(await screen.findByRole("menuitem", { name: `Unfollow ${bob}` }));

    const dialog = await screen.findByRole("alertdialog");
    expect(changes).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Unfollow" }));

    expect(await findToast(`You no longer follow ${isolated("Bob")}.`)).toBeInTheDocument();
    expect(changes).toEqual([`${otherAccountFixture.id} unfollow`]);
  });

  test("offers no new follow of a remote author while the instance does not follow other servers", async () => {
    serve(remoteStatusFixture);
    serveRelationship({});
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({ ...instanceFixture, federation: { mode: "outbound" } }),
      ),
    );
    const { user } = await renderApp({ path: `/@carol@remote.example/status/${remoteStatusFixture.id}` });

    await openMenu(user);

    const carol = isolated("@carol");
    await waitFor(() => {
      expect(screen.getByRole("menuitem", { name: `Mute ${carol}` })).not.toHaveAttribute("aria-disabled");
    });
    expect(screen.queryByRole("menuitem", { name: `Follow ${carol}` })).not.toBeInTheDocument();
  });

  test("a failure says so in a toast", async () => {
    serve(bobPost);
    serveRelationship({});
    server.use(http.post(`${apiBase}/accounts/:id/follow`, () => problem(503, "about:blank", "Service Unavailable")));
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);
    await waitFor(() => {
      expect(screen.getByRole("menuitem", { name: `Follow ${bob}` })).not.toHaveAttribute("aria-disabled");
    });
    await user.click(screen.getByRole("menuitem", { name: `Follow ${bob}` }));

    expect(
      await findToast("Could not change your relationship with the account: Service Unavailable"),
    ).toBeInTheDocument();
  });
});

describe("muting and blocking", () => {
  test("mutes at once; the timeline reads its posts again and the toast says so", async () => {
    let muted = false;
    serveRelationship({});
    server.use(
      http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage(muted ? [] : [bobPost]))),
      http.post(`${apiBase}/accounts/:id/mute`, ({ params }) => {
        muted = true;
        return HttpResponse.json(relationshipFixture(String(params["id"]), { muting: true }));
      }),
    );
    const { user } = await renderApp({ path: "/home" });

    await openMenu(user);
    await waitFor(() => {
      expect(screen.getByRole("menuitem", { name: `Mute ${bob}` })).not.toHaveAttribute("aria-disabled");
    });
    await user.click(screen.getByRole("menuitem", { name: `Mute ${bob}` }));

    expect(await findToast(`You muted ${isolated("Bob")}.`)).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "More actions" })).not.toBeInTheDocument();
    });
  });

  test("unmutes at once", async () => {
    serve(bobPost);
    serveRelationship({ muting: true });
    const changes = recordChanges({});
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);
    await user.click(await screen.findByRole("menuitem", { name: `Unmute ${bob}` }));

    await waitFor(() => {
      expect(changes).toEqual([`${otherAccountFixture.id} unmute`]);
    });
  });

  test("blocking asks first, as on a profile", async () => {
    serve(bobPost);
    serveRelationship({});
    const changes = recordChanges({ blocking: true });
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);
    await waitFor(() => {
      expect(screen.getByRole("menuitem", { name: `Block ${bob}` })).not.toHaveAttribute("aria-disabled");
    });
    await user.click(screen.getByRole("menuitem", { name: `Block ${bob}` }));

    const dialog = await screen.findByRole("alertdialog");
    expect(changes).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Block" }));

    expect(await findToast(`You blocked ${isolated("Bob")}.`)).toBeInTheDocument();
    expect(changes).toEqual([`${otherAccountFixture.id} block`]);
  });

  test("a blocked author offers only the unblock, which asks first", async () => {
    serve(bobPost);
    serveRelationship({ blocking: true });
    const changes = recordChanges({});
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);
    const unblock = await screen.findByRole("menuitem", { name: `Unblock ${bob}` });

    expect(screen.queryByRole("menuitem", { name: `Follow ${bob}` })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: `Direct message ${bob}` })).not.toBeInTheDocument();

    await user.click(unblock);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Unblock" }));

    await waitFor(() => {
      expect(changes).toEqual([`${otherAccountFixture.id} unblock`]);
    });
  });
});

describe("a direct message", () => {
  test("opens the form with a mention of the author and the direct visibility", async () => {
    serve(bobPost);
    serveRelationship({});
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);
    await user.click(await screen.findByRole("menuitem", { name: `Direct message ${bob}` }));

    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByRole("textbox", { name: /^(?!Search)/ })).toHaveValue("@bob ");
    expect(within(sheet).getByRole("button", { name: "Visibility: Mentioned people" })).toBeInTheDocument();
    await waitFor(() => {
      expect(within(sheet).getByRole("textbox", { name: /^(?!Search)/ })).toHaveFocus();
    });
  });

  test("names a remote author by the whole handle", async () => {
    serve(remoteStatusFixture);
    serveRelationship({});
    const { user } = await renderApp({ path: `/@carol@remote.example/status/${remoteStatusFixture.id}` });

    await openMenu(user);
    await user.click(await screen.findByRole("menuitem", { name: `Direct message ${isolated("@carol")}` }));

    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByRole("textbox", { name: /^(?!Search)/ })).toHaveValue("@carol@remote.example ");
  });
});

describe("who sees what", () => {
  test("the viewer's own post offers nothing about its author", async () => {
    const requests = recordRelationshipRequests();
    serve(ownPost);
    server.use(operatorHandler(["manage_accounts"]));
    const { user } = await renderApp({ path: `/@${viewerFixture.acct}/status/${ownPost.id}` });

    await openMenu(user);

    expect(await screen.findByRole("menuitem", { name: "Delete" })).toBeInTheDocument();
    for (const name of [/^Follow/, /^Direct message/, /^Mute (?!this thread)/, /^Block/, /in moderation$/, /^Report/]) {
      expect(screen.queryByRole("menuitem", { name })).not.toBeInTheDocument();
    }
    expect(requests).toEqual([]);
  });

  test("while signed out, no actions show at all", async () => {
    server.resetHandlers();
    serve(bobPost);
    await renderApp({ path: bobPath });

    expect(await screen.findByText("hello")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "More actions" })).not.toBeInTheDocument();
  });

  test("a viewer without manage_accounts has no moderation links", async () => {
    serve(bobPost);
    serveRelationship({});
    server.use(operatorHandler(["manage_reports", "view_audit"]));
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);

    expect(await screen.findByRole("menuitem", { name: `Mute ${bob}` })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /in moderation$/ })).not.toBeInTheDocument();
  });

  test("a moderator's links open the account's moderation page, the second from the post", async () => {
    serve(bobPost);
    serveRelationship({});
    server.use(operatorHandler(["manage_accounts"]));
    const { user } = await renderApp({ path: bobPath });

    await openMenu(user);

    const accountPath = `/moderation/accounts/${otherAccountFixture.id}`;
    expect(await screen.findByRole("menuitem", { name: "Open this account in moderation" })).toHaveAttribute(
      "href",
      accountPath,
    );
    expect(screen.getByRole("menuitem", { name: "Open this post in moderation" })).toHaveAttribute(
      "href",
      `${accountPath}?status=${bobPost.id}`,
    );
  });

  test("the menu of a boost is about the boosted post's author", async () => {
    const boost: Status = { ...viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000a4" }), boost: bobPost };
    const requests = recordRelationshipRequests();
    serveRelationship({});
    server.use(http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage([boost]))));
    const { user } = await renderApp({ path: "/home" });

    await openMenu(user);

    expect(await screen.findByRole("menuitem", { name: `Mute ${bob}` })).toBeInTheDocument();
    await waitFor(() => {
      expect(requests).toEqual([[otherAccountFixture.id]]);
    });
  });
});
