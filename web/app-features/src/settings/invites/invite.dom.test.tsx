// SPDX-License-Identifier: AGPL-3.0-only

// An invite's screen, under the settings' Invites (#3207, ADR-0106 §16–§18): titled by the invite's code; its link to
// send and to copy only while the invite works now; its state as the list says it; revoking it, offered while it
// could still work (usable, or not usable only while its creator may not create invites or sign-ups are closed),
// asked first and said in the confirmation when the server refuses; an address that names none of the reader's
// invites; an invite of a later page of the list, which the screen reads on to; and the screen while it reads and
// when it could not. That the list shows the invite revoked once the reader is back is the app's (web/app,
// settings-invites.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { fakeOrigin } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { clipboard, refusingClipboard } from "../../test/apps";
import { headings, heldInvite, inviteDrawn, inviteRead, loading, notLoaded, sendThis } from "../../test/invites";
import {
  buttonsOf,
  closed,
  dialogs,
  hourOf,
  inOrder,
  offered,
  row,
  rows,
  section,
  sections,
  words,
} from "../../test/security";

const revokeRow = "Stop this invite from working";
const revoking =
  "Its link and its code stop working at once, and this cannot be undone. People who signed up with it keep their accounts.";

/** The confirmation that asks before the invite of `code` is revoked, once it is open. */
const asking = (code: string) => screen.findByRole("alertdialog", { name: `Revoke the invite ${code}?` });

/** The items of the list named `name`: the link to send. */
const kept = (name: string) =>
  screen.queryAllByRole("list", { name }).flatMap((list) => within(list).getAllByRole("listitem").map(words));

/** What the section of the link says of a copy: an empty status is the place kept for it. */
const saidOfCopy = () => words(within(section("Invite link")).getByRole("status"));

describe("an invite reads as its state says", () => {
  const closedServer = { registrations: "closed" } as const;
  const notAllowed = { invites: "not-allowed" } as const;

  for (const [what, index, access, server, parts, says] of [
    [
      "a usable invite: its link, its state, and revoking it",
      0,
      {},
      {},
      ["Invite link", "Status", "Revoke invite"],
      (until: string) => `Usable until ${until} Used 0 of 1 time`,
    ],
    [
      "a usable invite used once of five",
      1,
      {},
      {},
      ["Invite link", "Status", "Revoke invite"],
      (until: string) => `Usable until ${until} Used 1 of 5 times`,
    ],
    ["a revoked invite: its state alone", 2, {}, {}, ["Status"], () => "Revoked Used 0 of 3 times"],
    [
      "an expired invite: its state alone",
      3,
      {},
      {},
      ["Status"],
      (until: string) => `Expired ${until} Used 1 of 1 time`,
    ],
    ["an invite used up: its state alone", 4, {}, {}, ["Status"], () => "Used up Used 4 of 4 times"],
    [
      "an invite the server reports as not usable while its creator is not eligible: no link, and it can be revoked",
      5,
      {},
      {},
      ["Status", "Revoke invite"],
      (until: string) => `Not usable while you cannot create invites Used 0 of 2 times Expires ${until}`,
    ],
    [
      "an invite of a reader who may not create invites now: no link, and it can be revoked",
      0,
      notAllowed,
      {},
      ["Status", "Revoke invite"],
      (until: string) => `Not usable while you cannot create invites Used 0 of 1 time Expires ${until}`,
    ],
    [
      "an invite that would work, on a server that takes no new account: no link, and it can be revoked",
      0,
      {},
      closedServer,
      ["Status", "Revoke invite"],
      (until: string) => `Not usable while sign-ups are closed Used 0 of 1 time Expires ${until}`,
    ],
  ] as const)
    test(what, async () => {
      const { fake } = await inviteRead(index, access, { server });
      const invite = await heldInvite(fake, index);

      // Its title is its code, then its sections.
      expect(headings()).toEqual([invite.code, ...parts]);
      expect(sections()).toEqual([...parts]);
      expect(rows("Status")).toEqual([row(says(hourOf(invite.expires_at)))]);
      expect(settingsFindings(document.body)).toEqual([]);
    });
});

describe("the link of an invite that works", () => {
  test("stands under what to do with it, which names the code, as the address of the sign-up page with the code, over Copy link", async () => {
    const { fake } = await inviteRead(0);
    const invite = await heldInvite(fake, 0);
    const link = `${fakeOrigin}/auth/sign_up?invite=${invite.code}`;

    expect(invite.url).toBe(link);
    expect(kept("Invite link")).toEqual([link]);
    expect(words(section("Invite link"))).toMatch(inOrder("Invite link", sendThis(invite.code), link, "Copy link"));
    expect(saidOfCopy()).toBe("");
  });

  test("Copy link copies the link, and the section says so", async () => {
    const { fake, user } = await inviteRead(0);
    const invite = await heldInvite(fake, 0);

    await user.click(within(section("Invite link")).getByRole("button", { name: "Copy link" }));

    await waitFor(() => {
      if (saidOfCopy() === "") throw new Error("test: the section says nothing of the copy yet");
    });
    expect(saidOfCopy()).toBe("Copied");
    expect(await clipboard().readText()).toBe(invite.url);
  });

  test("when the browser does not copy, the section says to select the link and copy it", async () => {
    const { user } = await inviteRead(0);
    refusingClipboard();

    await user.click(within(section("Invite link")).getByRole("button", { name: "Copy link" }));

    await waitFor(() => {
      if (saidOfCopy() === "") throw new Error("test: the section says nothing of the copy yet");
    });
    expect(saidOfCopy()).toBe("Could not copy. Select the text and copy it yourself.");
  });
});

describe("revoking", () => {
  test("Revoke asks first, naming the invite and what revoking does, and once confirmed the invite is revoked and the screen gives way to the list", async () => {
    const { fake, user, address } = await inviteRead(0);
    const invite = await heldInvite(fake, 0);
    const sent = fake.script("POST /invites/{id}/revoke");
    expect(rows("Revoke invite")).toEqual([row(`${revokeRow} ${revoking}`, { offers: ["Revoke"] })]);

    await user.click(offered("Revoke invite", revokeRow, "Revoke"));

    const dialog = await asking(invite.code);
    expect(words(dialog)).toContain(revoking);
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Revoke"]);
    expect(sent.seen).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await closed();
    await waitFor(() => {
      if (address.backs === 0) throw new Error("test: the screen did not ask for its way back yet");
    });
    expect(address.backs).toBe(1);
    expect(sent.seen.map((seen) => seen.url.pathname.split("/").at(-2))).toEqual([invite.id]);
    expect((await heldInvite(fake, 0)).state).toBe("revoked");
  });

  test("a revocation the server refuses stays in the confirmation, which says the server's words, and the reader stays on the invite", async () => {
    const { fake, user, address } = await inviteRead(0);
    const invite = await heldInvite(fake, 0);
    fake.script("POST /invites/{id}/revoke", {
      fail: { type: "about:blank", title: "The server could not revoke the invite", status: 500 },
      times: 1,
    });

    await user.click(offered("Revoke invite", revokeRow, "Revoke"));
    await user.click(within(await asking(invite.code)).getByRole("button", { name: "Revoke" }));

    const refused = await screen.findByRole("alert");
    expect(words(refused)).toBe("The server could not revoke the invite");
    expect((await asking(invite.code)).contains(refused)).toBe(true);
    expect(address.backs).toBe(0);
    expect(rows("Status")).toEqual([row(`Usable until ${hourOf(invite.expires_at)} Used 0 of 1 time`)]);

    // The same button tries again, and this time the server takes it.
    await user.click(within(await asking(invite.code)).getByRole("button", { name: "Revoke" }));

    await closed();
    await waitFor(() => {
      if (address.backs === 0) throw new Error("test: the screen did not ask for its way back yet");
    });
    expect(address.backs).toBe(1);
  });

  test("Cancel revokes nothing, and gives the focus back to Revoke", async () => {
    const { fake, user, address } = await inviteRead(0);
    const invite = await heldInvite(fake, 0);
    const sent = fake.script("POST /invites/{id}/revoke");
    const button = offered("Revoke invite", revokeRow, "Revoke");

    await user.click(button);
    await user.click(within(await asking(invite.code)).getByRole("button", { name: "Cancel" }));

    await closed();
    await waitFor(() => {
      if (document.activeElement !== button) throw new Error("test: the focus is not back on the button yet");
    });
    expect(sent.seen).toHaveLength(0);
    expect(address.backs).toBe(0);
    expect(dialogs()).toHaveLength(0);
  });
});

describe("an address", () => {
  const missing = /^This invite is not among your invites\. Check the address\.\s*See your invites$/;

  test("that names none of the reader's invites says so under the list's name, and leads to the list", async () => {
    const { data } = await inviteDrawn(99);

    const alert = await screen.findByRole("alert");
    expect(words(alert)).toMatch(missing);
    expect(within(alert).getByRole("link", { name: "See your invites" }).getAttribute("href")).toBe(
      "/settings/invites",
    );
    expect(headings()).toEqual(["Your invites"]);
    expect(data.queryClient.isFetching()).toBe(0);
  });

  test("of an invite on a later page of the list is found: the screen reads the list on until it has it", async () => {
    const { fake, requests } = await inviteRead(25, { invites: "many" });
    const invite = await heldInvite(fake, 25);

    expect(headings()).toEqual([invite.code, "Status"]);
    expect(rows("Status")).toEqual([row(`Expired ${hourOf(invite.expires_at)} Used 1 of 2 times`)]);
    expect(requests.filter((request) => request.startsWith("GET /invites"))).toHaveLength(2);
  });

  test("that names no invite among more than a page of them says so only once the whole list is read", async () => {
    const { requests } = await inviteDrawn(99, { invites: "many" });

    expect(words(await screen.findByRole("alert"))).toMatch(missing);
    expect(requests.filter((request) => request.startsWith("GET /invites"))).toHaveLength(2);
  });
});

describe("until the invite is read", () => {
  test("the screen says that it loads the settings, and shows the invite once the server answered", async () => {
    let held: { release: () => void } | undefined;
    const { fake } = await inviteDrawn(
      0,
      {},
      {
        before: (on) => {
          held = on.fake.script("GET /invites", { hold: "request" });
        },
      },
    );
    expect((await screen.findByRole("status")).textContent).toBe(loading);
    expect(sections()).toEqual([]);

    held?.release();

    await screen.findByRole("region", { name: "Status" });
    expect(headings()[0]).toBe((await heldInvite(fake, 0)).code);
  });

  test("the screen says that the settings could not be loaded when the list fails, and Try again reads the invite", async () => {
    const { user } = await inviteDrawn(
      0,
      {},
      {
        before: (on) => {
          on.fake.script("GET /invites", { fail: 500, times: 1 });
        },
      },
    );

    const failed = await screen.findByRole("alert");
    expect(words(failed)).toMatch(notLoaded);
    expect(sections()).toEqual([]);

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await screen.findByRole("region", { name: "Status" });
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });
});
