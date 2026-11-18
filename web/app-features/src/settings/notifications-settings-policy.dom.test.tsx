// SPDX-License-Identifier: AGPL-3.0-only

// Notifications, the entry of the settings (#3201): the policy for notifications from people the reader may not know
// (ADR-0158 §10), where each of five conditions takes one of three actions with a PATCH
// /preferences/notification_policy of that condition alone, and the screen says what each action does; and, on a
// server that stopped sending email to the reader's address (ADR-0086 §5, §8), the summary that says why first, with
// the way to verify the address again after a bounce, and what the server answered to it.

import { describe, expect, test } from "bun:test";
import type { NotificationPolicy } from "@my/app-data";
import { memberNotificationRows, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { describedBy, people, readSettings, said, section, sections } from "../test/notification-settings";

const member = { email: true, settings: { notificationRows: memberNotificationRows } } as const satisfies FakeOptions;

const unknown = "Notifications from people you may not know";

/** The conditions of the policy, in the order of the screen: the server's name of each, and the screen's. */
const conditions = [
  ["not_following", "People you do not follow"],
  ["not_follower", "People who do not follow you"],
  ["new_account", "New accounts"],
  ["unsolicited_private_mention", "Unrequested direct messages"],
  ["limited_account", "Accounts limited by the moderators"],
] as const satisfies readonly (readonly [keyof NotificationPolicy, string])[];

/** The choice of the condition named `name`. */
const choice = (name: string) => section(unknown).getByRole<HTMLSelectElement>("combobox", { name });
/** What the choice of the condition named `name` reads. */
const chosen = (name: string) => choice(name).selectedOptions[0]?.textContent;
/** What the rows of the policy say of a save. */
const saves = () => said(section(unknown).queryAllByRole("status"));

describe("notifications from people the reader may not know", () => {
  test("the screen says what each of the three choices does, before the choices", async () => {
    await readSettings(member);

    const list = section(unknown).getByRole("list", { name: "What each choice does" });
    const [accept, hold, drop, ...others] = within(list)
      .getAllByRole("listitem")
      .map((item) => item.textContent);
    expect(others).toEqual([]);
    expect(accept).toMatch(/^Accept\s*The notification arrives as usual\.$/);
    expect(hold).toMatch(/^Hold back\s*It waits in “Held-back notifications” until you accept or dismiss it\.$/);
    expect(drop).toMatch(/^Drop\s*It is never created, so you cannot see it later\.$/);
    const [first] = section(unknown).getAllByRole("combobox");
    if (first === undefined) throw new Error("test: the section has no choice");
    expect(list.compareDocumentPosition(first) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  });

  test("each of the five conditions is a choice of the three actions, at what the server holds", async () => {
    await readSettings({
      ...member,
      settings: {
        ...member.settings,
        policy: {
          not_following: "accept",
          not_follower: "filter",
          new_account: "drop",
          unsolicited_private_mention: "filter",
          limited_account: "accept",
        },
      },
    });

    expect(
      section(unknown)
        .getAllByRole("combobox")
        .map((select) => select.getAttribute("aria-labelledby"))
        .map((id) => document.getElementById(id ?? "")?.textContent),
    ).toEqual(conditions.map(([, name]) => name));
    for (const [, name] of conditions)
      expect([
        name,
        within(choice(name))
          .getAllByRole("option")
          .map((option) => option.textContent),
      ]).toEqual([name, ["Accept", "Hold back", "Drop"]]);
    expect(conditions.map(([, name]) => chosen(name))).toEqual(["Accept", "Hold back", "Drop", "Hold back", "Accept"]);
  });

  test("a condition that needs it says whom it means", async () => {
    await readSettings(member);

    expect(describedBy(choice("New accounts"))).toEqual(["Accounts created less than 30 days ago."]);
    expect(describedBy(choice("Unrequested direct messages"))).toEqual([
      "Direct messages that do not reply to one you sent to that person.",
    ]);
    expect(describedBy(choice("Accounts limited by the moderators"))).toEqual([
      "Accounts the moderators of this server limited, by themselves or with their whole server. This applies to people you follow too.",
    ]);
  });

  for (const [condition, name] of conditions)
    test(`“${name}” saves at once, with a PATCH of that condition alone`, async () => {
      const { fake, user } = await readSettings(member);
      const sent = fake.script("PATCH /preferences/notification_policy");

      await user.selectOptions(choice(name), "Drop");

      expect((await sent.served()).body).toEqual({ [condition]: "drop" });
      await waitFor(() => {
        expect(saves()).toEqual(["Saved"]);
      });
      expect(sent.seen).toHaveLength(1);
      expect(chosen(name)).toBe("Drop");
    });

  test("Hold back is sent as the server's filter, and Accept as its accept", async () => {
    const { fake, user } = await readSettings(member);
    const sent = fake.script("PATCH /preferences/notification_policy");

    await user.selectOptions(choice("People you do not follow"), "Hold back");
    await sent.served();
    await user.selectOptions(choice("New accounts"), "Accept");
    await sent.served(2);

    expect(sent.seen.map((request) => request.body)).toEqual([{ not_following: "filter" }, { new_account: "accept" }]);
  });

  test("a choice says Saving… until the server answers, and one the server fails goes back with Try again, which sends it again", async () => {
    const { fake, user } = await readSettings(member);
    const sent = fake.script("PATCH /preferences/notification_policy");
    const failing = fake.script("PATCH /preferences/notification_policy", { times: 1, hold: "request", fail: 500 });
    expect(saves()).toEqual([]);

    await user.selectOptions(choice("People who do not follow you"), "Hold back");
    await failing.arrived();
    expect(saves()).toEqual(["Saving…"]);

    failing.release();
    await waitFor(() => {
      expect(saves().join()).toMatch(/^This could not be saved\.\s*Try again$/);
    });
    expect(chosen("People who do not follow you")).toBe("Accept");
    failing.end();

    await user.click(section(unknown).getByRole("button", { name: "Try again" }));

    await waitFor(() => {
      expect(saves()).toEqual(["Saved"]);
    });
    expect(sent.seen.map((request) => request.body)).toEqual([{ not_follower: "filter" }, { not_follower: "filter" }]);
    expect(chosen("People who do not follow you")).toBe("Hold back");
  });

  test("the section ends with the way to the held-back notifications", async () => {
    await readSettings(member);

    const [link, ...others] = section(unknown).getAllByRole("link");
    if (link === undefined) throw new Error("test: the section has no link");
    expect(others).toEqual([]);
    expect(link.textContent).toBe("Held-back notifications");
    expect(link.getAttribute("href")).toBe("/notifications/requests");
    for (const select of section(unknown).getAllByRole("combobox"))
      expect(select.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  });
});

describe("email to the reader's address is stopped", () => {
  const stopped = "Email to your address is stopped";
  const route = "POST /accounts/email/verification";
  const send = "Send the verification email";

  /** A server that stopped sending email to the reader's address, for the reason `kind`. */
  const stoppedFor = (
    kind: NonNullable<NonNullable<FakeOptions["preferences"]>["email_suppression"]>["kind"],
    more: FakeOptions = {},
  ): FakeOptions => ({
    ...member,
    ...more,
    preferences: { email_suppression: { kind, state_changed_at: "2026-11-02T09:30:00Z" } },
  });

  /** What the summary says of the email asked for. */
  const answers = () => said(section(stopped).queryAllByRole("status"));

  test("the screen says nothing of it while the server sends email to the address", async () => {
    await readSettings(member);

    expect(sections()).not.toContain(stopped);
    expect(screen.queryAllByRole("button", { name: send })).toHaveLength(0);
  });

  test("nor on a server that sends no email at all, whatever it holds of the address", async () => {
    await readSettings(stoppedFor("hard_bounce", { email: false }));

    expect(sections()).not.toContain(stopped);
    expect(screen.queryAllByRole("button", { name: send })).toHaveLength(0);
  });

  test("after an email that could not be delivered, a summary stands first, says why, and offers to verify the address again", async () => {
    await readSettings(stoppedFor("hard_bounce"));

    expect(sections()).toEqual([
      stopped,
      "This device",
      people,
      "From this server",
      "Notifications from people you may not know",
    ]);
    expect(
      section(stopped).getAllByText(
        "An email to your address could not be delivered, so this server sends you no email, notifications included, until you verify the address again.",
      ),
    ).toHaveLength(1);
    expect(section(stopped).getAllByRole("button", { name: send })).toHaveLength(1);
    expect(answers()).toEqual([]);
  });

  test("after a complaint, the summary says that the moderators can lift it, and offers no email", async () => {
    await readSettings(stoppedFor("complaint"));

    expect(sections()[0]).toBe(stopped);
    const [words, ...others] = section(stopped).getAllByText(/marked as spam/);
    expect(others).toEqual([]);
    expect(words?.textContent).toMatch(/To receive them again, ask the moderators of this server\.$/);
    expect(section(stopped).queryAllByRole("button")).toHaveLength(0);
  });

  test("Send the verification email asks the server once, and says that it was sent", async () => {
    const { fake, user, requests } = await readSettings(stoppedFor("hard_bounce"));
    const sent = fake.script(route);

    await user.click(section(stopped).getByRole("button", { name: send }));

    await sent.served();
    await waitFor(() => {
      expect(answers()).toEqual(["Sent. Open the link in the email to receive email again."]);
    });
    expect(requests.filter((request) => !request.startsWith("GET "))).toEqual(["POST /accounts/email/verification"]);
  });

  test("an address the server says is verified already (a 409): a status in place of the summary says that email is on again, and stays once the preferences are read again", async () => {
    const verified = "Your address is already verified. Email to it is on again.";
    const { fake, user, data } = await readSettings(stoppedFor("hard_bounce"));
    fake.script(route, {
      fail: { type: "/api/v1/problems/email-already-verified", title: "The address is verified already", status: 409 },
    });
    // The server holds the address verified: the preferences read again say nothing of a stop.
    const reread = fake.script("GET /preferences", {
      change: ({ email_suppression: _stop, ...preferences }) => preferences,
    });

    await user.click(section(stopped).getByRole("button", { name: send }));

    await reread.served();
    await waitFor(() => {
      expect(said(screen.queryAllByRole("status"))).toContain(verified);
      expect(data.queryClient.isFetching()).toBe(0);
    });
    expect(screen.queryAllByRole("region", { name: stopped })).toHaveLength(0);
    expect(screen.queryAllByRole("button", { name: send })).toHaveLength(0);
    expect(said(screen.queryAllByRole("status")).filter((words) => words === verified)).toHaveLength(1);
  });

  const refusals: [string, FakeOptions, number | undefined, string][] = [
    [
      "a server that sent too many says how long to wait, from its Retry-After",
      { verificationEmail: "limited" },
      undefined,
      "You asked for the email too many times. Try again in 25 minutes.",
    ],
    [
      "a server that sent too many and names no wait says to try later",
      {},
      429,
      "You asked for the email too many times. Try again later.",
    ],
    ["any other failure says that the email could not be sent", {}, 500, "The email could not be sent. Try again."],
  ];

  for (const [what, options, fail, says] of refusals)
    test(what, async () => {
      const { fake, user } = await readSettings(stoppedFor("hard_bounce", options));
      if (fail !== undefined) fake.script(route, { fail });

      await user.click(section(stopped).getByRole("button", { name: send }));

      await waitFor(() => {
        expect(answers()).toEqual([says]);
      });
      // The reader can ask again.
      expect(section(stopped).getAllByRole("button", { name: send })).toHaveLength(1);
    });
});
