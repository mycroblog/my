// SPDX-License-Identifier: AGPL-3.0-only

// The host's side of a voice chat room on the fake API with the calls on fakes (ADR-0100 §2, §3, §5, §7, §9, §13):
// who is offered to open a room; the server's three refusals of an opening, said before the form's button; the start
// post, offered for the rooms of the whole server and of the host's followers only, its draft holding the room's name
// until the host writes it, and its text sent with the room; the co-hosts, three at most; the invitations, as a
// listener and as a speaker, revoked, and refused; the settings a host saves; and the rooms one hosted, each with when
// it opened, ended and why, the open one leading to its room.

import { openCall } from "@my/app-features";
import { describe, expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { callsOnFakes, type FakeCalls } from "./test/call";
import { fakeApp } from "./test/server";

/** The name the room's tiles and lists call the participant of `username`. */
async function nameOf(app: TestApp, room: string, username: string): Promise<string> {
  const { data } = await app.data.api.GET("/voice/rooms/{id}", { params: { path: { id: room } } });
  const p = data?.participants.find((q) => q.account.username === username);
  if (p === undefined) throw new Error(`test: ${username} is not in the room`);
  return p.account.display_name || p.account.username;
}

/** Renders the fake's room the reader hosts (others in it, Hana its co-host), and joins it as Join does. */
async function hosting(calls: FakeCalls): Promise<{ app: TestApp; id: string }> {
  const on = fakeApp();
  const id = on.fake.voice.roomId(2);
  const app = await renderApp(`/voice/${id}`, { on });
  await app.user.click(await main().findByRole("button", { name: "Join" }));
  await calls.clock.advance(0);
  await screen.findByRole("toolbar", { name: "Room controls" });
  return { app, id };
}

/** The tile of the participant named `name`, a button for a host who may act on them. */
const tile = (name: string) => main().getByRole("button", { name: new RegExp(`^${name}(?:, .*)?$`) });

/** Writes `text` at the end of the field `field`. */
async function write(app: TestApp, field: HTMLElement, text: string): Promise<void> {
  await app.user.click(field);
  await app.user.paste(text);
}

/** The names of the start post's checkbox: posted to the whole server, or to the host's followers. */
const startPostNames = ["Post to this server that the room is open", "Post to your followers that the room is open"];

/** The form's fields, once it is drawn. */
async function form() {
  return {
    name: await main().findByRole("textbox", { name: "Name" }),
    /** The start post's checkbox, by its name, which says who it is posted to; none when it is not offered. */
    startPost: () => {
      const named = startPostNames.filter((name) => main().queryAllByRole("checkbox", { name }).length > 0);
      // The form has no other checkbox.
      expect(main().queryAllByRole("checkbox")).toHaveLength(named.length);
      return named;
    },
    post: () => main().getByRole<HTMLTextAreaElement>("textbox", { name: "Text of the post" }),
    submit: main().getByRole("button", { name: "Open and join" }),
  };
}

/** Chooses `label` of the radio group `group`, by its label's words, as a reader does. */
async function choose(app: TestApp, group: string, label: string): Promise<void> {
  await app.user.click(within(main().getByRole("radiogroup", { name: group })).getByText(label, { exact: true }));
}

describe("who is offered to open a room", () => {
  test.each([
    ["everyone may (the server's choice here)", {}, false, true],
    ["only the permitted may, and the reader is not", { voiceStarters: "permitted" as const }, false, false],
    ["only the permitted may, and the reader is", { voiceStarters: "permitted" as const }, true, true],
  ])("%s", async (_, options, permitted, offered) => {
    const on = fakeApp(options);
    if (permitted)
      on.fake.script("GET /accounts/verify_credentials", {
        change: (account) => ({ ...account, permissions: [...account.permissions, "start_voice_rooms" as const] }),
      });
    const app = await renderApp("/voice", { on });
    // The rooms are listed, and nothing is on its way.
    await waitFor(() => {
      expect(
        main()
          .queryAllByRole("link")
          .some((link) => words(link).includes("Morning coffee chat")),
      ).toBe(true);
      expect(app.data.queryClient.isFetching()).toBe(0);
    });
    const open = main().queryAllByRole("link", { name: "Open a room" });
    expect(open.map((link) => link.getAttribute("href"))).toEqual(offered ? ["/voice/new"] : []);
  });

  test("the voice chat off on the server: nobody", async () => {
    const app = await renderApp("/voice", { voiceOff: true });
    expect(await main().findByText("Voice chat is not available on this server.")).toBeDefined();
    await waitFor(() => {
      expect(app.data.queryClient.isFetching()).toBe(0);
    });
    expect(main().queryAllByRole("link", { name: "Open a room" })).toHaveLength(0);
  });
});

describe("the server refuses to open the room, said before the form's button", () => {
  test.each([
    ["not-eligible" as const, "You cannot open rooms on this server."],
    ["too-many-rooms" as const, "As many rooms are open as this server allows. Try again later."],
    ["hosting-elsewhere" as const, "You host another room. End it or hand it over before you open this one."],
  ])("%s", async (voiceOpen, says) => {
    callsOnFakes();
    const app = await renderApp("/voice/new", { voiceOpen });
    const fields = await form();
    await write(app, fields.name, "Alpha");
    await app.user.click(fields.submit);

    const alert = await waitFor(() => {
      const said = main()
        .getAllByRole("alert")
        .find((element) => words(element) === says);
      if (said === undefined) throw new Error("test: the refusal is not said yet");
      return said;
    });
    expect(alert.compareDocumentPosition(fields.submit) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(app.path()).toBe("/voice/new");
    expect(app.fake.voice.joined()).toBeNull();
  });
});

describe("the start post", () => {
  test("is offered for the rooms of the whole server and of the host's followers, and no other", async () => {
    callsOnFakes();
    const app = await renderApp("/voice/new");
    const fields = await form();
    expect(fields.startPost()).toEqual(["Post to this server that the room is open"]);
    const offered: Record<string, readonly string[]> = {};
    for (const label of [
      "Your followers",
      "People you follow",
      "People you follow who follow you back",
      "Only invited people",
      "Everyone on this server",
    ]) {
      await choose(app, "Who can see the room", label);
      // The choice was taken: its radio is checked ("Your followers" is also a choice of who speaks, not checked).
      expect(main().getAllByRole("radio", { name: label, checked: true })).toHaveLength(1);
      offered[label] = fields.startPost();
    }
    expect(offered).toEqual({
      "Your followers": ["Post to your followers that the room is open"],
      "People you follow": [],
      "People you follow who follow you back": [],
      "Only invited people": [],
      "Everyone on this server": ["Post to this server that the room is open"],
    });
  });

  test("its draft holds the room's name as it is written, until the host writes the post", async () => {
    callsOnFakes();
    const app = await renderApp("/voice/new");
    const fields = await form();
    await write(app, fields.name, "Alpha");
    expect(fields.post().value).toBe("I opened the voice chat room “Alpha”.");
    await write(app, fields.name, " Beta");
    expect(fields.post().value).toBe("I opened the voice chat room “Alpha Beta”.");

    await app.user.clear(fields.post());
    await write(app, fields.post(), "Come and talk");
    await write(app, fields.name, " Gamma");
    expect(fields.post().value).toBe("Come and talk");
  });

  test("its text goes out with the room", async () => {
    callsOnFakes();
    const app = await renderApp("/voice/new");
    const fields = await form();
    await write(app, fields.name, "Alpha");
    await app.user.clear(fields.post());
    await write(app, fields.post(), "Come and talk about Alpha");
    await app.user.click(fields.submit);

    await waitFor(() => {
      expect(app.path()).toMatch(/^\/voice\/(?!new$)[^/]+$/);
    });
    const id = app.path().split("/").at(-1);
    expect(app.fake.voice.startPosts()).toEqual([{ room: id ?? "", text: "Come and talk about Alpha" }]);
  });

  test("a room for the people the host follows opens with none", async () => {
    callsOnFakes();
    const app = await renderApp("/voice/new");
    const fields = await form();
    await write(app, fields.name, "Alpha");
    await choose(app, "Who can see the room", "People you follow who follow you back");
    await app.user.click(fields.submit);

    await waitFor(() => {
      expect(app.path()).toMatch(/^\/voice\/(?!new$)[^/]+$/);
    });
    expect(app.fake.voice.startPosts()).toEqual([]);
  });
});

test("the host names co-hosts up to three, and then no one more", async () => {
  const calls = callsOnFakes();
  const { app, id } = await hosting(calls);
  // Hana co-hosts already: two more make three.
  for (const username of ["mio", "kai"]) {
    const name = await nameOf(app, id, username);
    await app.user.click(tile(name));
    await app.user.click(await screen.findByRole("menuitem", { name: "Make co-host" }));
    await main().findByRole("button", { name: new RegExp(`^${name}, Co-host(?:, .*)?$`) });
  }
  const ren = await nameOf(app, id, "ren");
  const cohost = app.fake.script("POST /voice/rooms/{id}/participants/{account_id}/cohost");
  await app.user.click(tile(ren));
  await screen.findByRole("menuitem", { name: "Remove from the room…" });
  const offered = screen
    .queryAllByRole("menuitem", { name: "Make co-host" })
    .filter((item) => item.getAttribute("aria-disabled") !== "true");
  expect(offered).toHaveLength(0);
  expect(cohost.seen).toHaveLength(0);
});

describe("the invitations", () => {
  /** The room's invitations, as the host opens them from the room. */
  async function invitations(calls: FakeCalls) {
    const { app, id } = await hosting(calls);
    await app.navigate(`/voice/${id}/invitations`);
    const search = await main().findByRole("searchbox", { name: "Name or handle" });
    /** Finds `handle` and presses Invite on them. */
    const invite = async (query: string, handle: string) => {
      await app.user.clear(search);
      await write(app, search, query);
      // The row of the search's results that names the handle and offers Invite.
      const found = await waitFor(() => {
        const row = main()
          .getAllByRole("listitem")
          .find(
            (item) =>
              words(item).includes(handle) && within(item).queryAllByRole("button", { name: "Invite" }).length > 0,
          );
        if (row === undefined) throw new Error(`test: ${handle} is not found yet`);
        return row;
      });
      await app.user.click(within(found).getByRole("button", { name: "Invite" }));
    };
    /** The invitations the server holds, by username and role. */
    const held = async () => {
      const { data } = await app.data.api.GET("/voice/rooms/{id}/invitations", { params: { path: { id } } });
      return (data?.items ?? []).map((i) => `${i.account.username} ${i.role}`);
    };
    return { app, id, invite, held };
  }

  test("invite one as a listener and another as a speaker, each listed with Revoke", async () => {
    const calls = callsOnFakes();
    const { app, invite, held } = await invitations(calls);
    await invite("mio", "@mio@");
    expect(await main().findByText("You invited Mio 澪 as a listener.")).toBeDefined();
    await choose(app, "Invite as", "As a speaker, who can speak without asking");
    await invite("sam", "@sam@");
    expect(await held()).toEqual(["mio listener", "sam speaker"]);

    await waitFor(() => {
      expect(main().getAllByRole("button", { name: "Revoke" })).toHaveLength(2);
    });
  });

  test("a revoked invitation leaves the list and the server", async () => {
    const calls = callsOnFakes();
    const { app, invite, held } = await invitations(calls);
    await invite("mio", "@mio@");
    const revoke = await main().findByRole("button", { name: "Revoke" });
    await app.user.click(revoke);

    expect(await main().findByText("You revoked the invitation of Mio 澪.")).toBeDefined();
    await waitFor(() => {
      expect(main().queryAllByRole("button", { name: "Revoke" })).toHaveLength(0);
    });
    expect(await held()).toEqual([]);
  });

  /** Waits until the screen says `says` in a status or an alert. */
  async function said(says: string): Promise<void> {
    await waitFor(() => {
      const regions = [...main().queryAllByRole("status"), ...main().queryAllByRole("alert")];
      expect(regions.map((region) => words(region)).filter((text) => text !== "")).toContain(says);
    });
  }

  test("one invited before, also revoked since, is not invited again", async () => {
    const calls = callsOnFakes();
    const { app, invite, held } = await invitations(calls);
    await invite("mio", "@mio@");
    await app.user.click(await main().findByRole("button", { name: "Revoke" }));
    await main().findByText("You revoked the invitation of Mio 澪.");
    await invite("mio", "@mio@");

    await said("Mio 澪 was invited to this room before. A person can be invited to a room once.");
    expect(await held()).toEqual([]);
  });

  test("one the server cannot invite (removed from the room) is said so", async () => {
    const calls = callsOnFakes();
    const { app, id, invite, held } = await invitations(calls);
    // Another device of the host removed Mio from the room meanwhile.
    await app.data.api.DELETE("/voice/rooms/{id}/participants/{account_id}", {
      params: { path: { id, account_id: app.fake.accountId("mio") } },
    });
    await invite("mio", "@mio@");

    await said("Mio 澪 cannot be invited to this room.");
    expect(await held()).toEqual([]);
  });

  test("past the room's invitations for the hour, the screen says to try again later", async () => {
    const calls = callsOnFakes();
    const { app, invite, held } = await invitations(calls);
    app.fake.script("POST /voice/rooms/{id}/invitations", {
      fail: { type: "about:blank", title: "Too Many Requests", status: 429 },
    });
    await invite("mio", "@mio@");

    await said("This room has sent as many invitations as it can for now. Try again later.");
    expect(await held()).toEqual([]);
  });
});

test("the host saves the room's new name, which the room then has", async () => {
  const calls = callsOnFakes();
  const { app, id } = await hosting(calls);
  await app.navigate(`/voice/${id}/settings`);
  const name = await main().findByRole("textbox", { name: "Name" });
  await app.user.clear(name);
  await write(app, name, "Photo walk, the second");
  await app.user.click(main().getByRole("button", { name: "Save" }));

  await waitFor(async () => {
    const { data } = await app.data.api.GET("/voice/rooms/{id}", { params: { path: { id } } });
    expect(data?.name).toBe("Photo walk, the second");
  });
  await app.navigate(`/voice/${id}`);
  expect(await main().findByRole("heading", { level: 2, name: /Photo walk, the second/ })).toBeDefined();
});

describe("the rooms one hosted", () => {
  /** The rows of the screen's one list, the rooms one hosted, as each reads and where it leads. */
  function hosted() {
    return main()
      .getAllByRole("listitem")
      .map((item) => ({
        says: words(item),
        leads: within(item)
          .queryAllByRole("link")
          .map((link) => link.getAttribute("href")),
      }));
  }

  test("each with when it opened and ended and why; the one open leads to its room, until its host ends it", async () => {
    callsOnFakes();
    const app = await renderApp("/lists");
    const call = await openCall(app.data, { name: "Evening talk" });
    const id = app.fake.voice.joined()?.room ?? "";
    await app.navigate("/settings/voice");
    await main().findByText(/^Sunday photo review/);

    const open = hosted();
    expect(open[0]?.says).toMatch(/^Evening talk\s*Opened .+$/);
    expect(open[0]?.says).not.toMatch(/Ended/);
    expect(open[0]?.leads).toEqual([`/voice/${id}`]);
    expect(open).toHaveLength(3);
    expect(open[1]?.says).toMatch(/^Sunday photo review\s*Opened .+\s*Ended .+\s*The host ended it$/);
    expect(open[2]?.says).toMatch(/^Trail report\s*Opened .+\s*Ended .+\s*Ended when everyone left$/);
    expect(open.slice(1).map((row) => row.leads)).toEqual([[], []]);

    await call.endRoom();
    await app.navigate("/lists");
    await app.navigate("/settings/voice");
    await waitFor(() => {
      const [first] = hosted();
      expect(first?.says).toMatch(/^Evening talk\s*Opened .+\s*Ended .+\s*The host ended it$/);
      expect(first?.leads).toEqual([]);
    });
  });
});
