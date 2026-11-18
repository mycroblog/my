// SPDX-License-Identifier: AGPL-3.0-only

// The screen that creates an invite, under the settings' Invites (#3207, ADR-0106 §15): a form saved as a whole, how
// many people may sign up with the invite (up to the server's most, at its default) and how long it works (usual
// lengths within the server's longest, at its default); what it sends; the invite it made, shown in the form's
// place with its link, until Finish; what it says when the server refuses (the limit of usable invites, a value the
// server no longer allows, an account that may not create invites, #3116); and what stands in the form's place for
// an account that may not create invites or holds as many as the server allows. Where the focus goes once the
// invite is made is jsdom's (focus.jsdom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import {
  createDrawn,
  createForm,
  headings,
  heldInvites,
  loading,
  notLoaded,
  refusalOn,
  sendThis,
} from "../../test/invites";
import { atRest, describedBy, hourOf, inOrder, row, rows, section, sections, words } from "../../test/security";
import type { Given } from "../../test/settings";

const cannot = "You cannot create invites";
const created = "The invite is created";
const hour = 3600;
const day = 24 * hour;

const people = () => screen.getByRole<HTMLInputElement>("textbox", { name: "People who can sign up" });
const life = () => screen.getByRole("combobox", { name: "Works for" });
const create = () => screen.getByRole<HTMLButtonElement>("button", { name: "Create invite" });

/** The lengths the form offers, and the one chosen. */
const lives = () => ({
  offered: within(life())
    .getAllByRole("option")
    .map((option) => option.textContent),
  chosen: within(life())
    .getAllByRole("option", { selected: true })
    .map((option) => option.textContent),
});

/** The controls of the form that are on the screen, by what they are. */
const controls = () => [
  ...screen.queryAllByRole("textbox").map(() => "number"),
  ...screen.queryAllByRole("combobox").map(() => "choice"),
  ...screen.queryAllByRole("button", { name: "Create invite" }).map(() => "submit"),
];

/** Writes `count` over the number of people. */
async function enter(user: UserEvent, count: string) {
  await user.click(people());
  await user.keyboard(`{Control>}a{/Control}${count}`);
}

/** Waits until the screen shows the invite it made. */
const made = () => screen.findByRole("region", { name: created });

/** A server whose limits for a member's invite are `limits`, laid over the fake's. */
const limited = (limits: {
  max_uses?: number;
  default_max_uses?: number;
  max_expires_in: number;
  default_expires_in: number;
}): Given => ({
  before: ({ fake }) => {
    fake.script("GET /invites", { change: (body) => ({ ...body, limits: { ...body.limits, ...limits } }) });
  },
});

describe("the form", () => {
  test("asks how many people can sign up, up to the server's most, and how long the invite works, each at the server's default, with one button that creates", async () => {
    await createForm();

    expect(headings()).toEqual(["Create invite", "New invite"]);
    expect(people().value).toBe("1");
    expect(describedBy(people())).toEqual(["Up to 10."]);
    expect(lives()).toEqual({
      offered: ["1 hour", "6 hours", "12 hours", "1 day", "3 days", "7 days", "14 days", "30 days"],
      chosen: ["7 days"],
    });
    expect(controls()).toEqual(["number", "choice", "submit"]);
    expect(create().getAttribute("type")).toBe("submit");
    expect(settingsFindings(document.body)).toEqual([]);
  });

  for (const [what, limits, uses, offered, chosen] of [
    [
      "a server that allows three people and two days",
      { max_uses: 3, default_max_uses: 2, max_expires_in: 2 * day, default_expires_in: 2 * day },
      ["2", "Up to 3."],
      ["1 hour", "6 hours", "12 hours", "1 day", "2 days"],
      "2 days",
    ],
    [
      "a server whose longest is between two usual lengths",
      { max_expires_in: 36 * hour, default_expires_in: 36 * hour },
      ["1", "Up to 10."],
      ["1 hour", "6 hours", "12 hours", "1 day", "36 hours"],
      "36 hours",
    ],
    [
      "a server whose longest is a year",
      { max_expires_in: 365 * day, default_expires_in: 7 * day },
      ["1", "Up to 10."],
      ["1 hour", "6 hours", "12 hours", "1 day", "3 days", "7 days", "14 days", "30 days", "90 days", "365 days"],
      "7 days",
    ],
  ] as const)
    test(`on ${what}, the number stops at the server's most and the lengths at its longest, each at its default`, async () => {
      await createForm({}, limited(limits));

      expect([people().value, ...describedBy(people())]).toEqual([...uses]);
      expect(lives()).toEqual({ offered: [...offered], chosen: [chosen] });
    });

  for (const [what, fill, body] of [
    ["left as it is, the server's defaults", () => Promise.resolve(), { max_uses: 1, expires_in: 7 * day }],
    [
      "what the reader chose",
      async (user: UserEvent) => {
        await enter(user, "3");
        await user.selectOptions(life(), "1 day");
      },
      { max_uses: 3, expires_in: day },
    ],
    [
      "the server's most where the reader entered more",
      async (user: UserEvent) => {
        await enter(user, "25");
      },
      { max_uses: 10, expires_in: 7 * day },
    ],
  ] as const)
    test(`Create invite sends, once, ${what}`, async () => {
      const { fake, user } = await createForm();
      const sent = fake.script("POST /invites");

      await fill(user);
      await user.click(create());

      await made();
      expect(sent.seen.map((seen) => seen.body)).toEqual([body]);
    });
});

describe("once the invite is made", () => {
  test("it stands in the form's place: that it is created and what to do with its link, the link to copy, its state, and Finish, which leads to the list", async () => {
    const { fake, user, address } = await createForm();
    const before = (await heldInvites(fake)).map((invite) => invite.id);

    await user.click(create());

    await made();
    const [invite, ...others] = (await heldInvites(fake)).filter((held) => !before.includes(held.id));
    if (invite === undefined) throw new Error("test: the fake holds no new invite");
    expect(others).toEqual([]);
    expect(headings()).toEqual(["Create invite", created, "Invite link", "Status"]);
    expect(words(section(created))).toMatch(inOrder(created, sendThis(invite.code)));
    expect(words(section("Invite link"))).toMatch(inOrder("Invite link", invite.url, "Copy link"));
    expect(rows("Status")).toEqual([row(`Usable until ${hourOf(invite.expires_at)} Used 0 of 1 time`)]);
    // The form is gone, and with it the way back that would leave the link unread: Finish is the way on.
    expect(controls()).toEqual([]);
    expect(
      within(screen.getByRole("main"))
        .getAllByRole("button")
        .map((button) => button.textContent),
    ).toEqual(["Copy link", "Finish"]);
    expect(settingsFindings(document.body)).toEqual([]);
    expect(address.backs).toBe(0);

    await user.click(screen.getByRole("button", { name: "Finish" }));

    expect(address.backs).toBe(1);
  });
});

describe("when the server refuses", () => {
  /** What the submit bar says of the refusal: an alert in its status. */
  const refused = async () => words(await screen.findByRole("alert"));

  test("at the limit of usable invites, the form says the server's sentence beside its button and keeps what the reader entered, which the next press sends again", async () => {
    const problem = await refusalOn({ access: { invites: "at-cap" } });
    expect([problem.status, problem.type]).toEqual([409, "/api/v1/problems/too-many-invites"]);
    const { fake, user } = await createForm();
    const sent = fake.script("POST /invites", { fail: problem, times: 1 });
    await enter(user, "3");
    await user.selectOptions(life(), "1 day");

    await user.click(create());

    // The server's own words: what it says of this refusal, or else of its kind.
    expect(await refused()).toBe(problem.detail ?? problem.title);
    // In the status of the submit bar, which stands in the form with its button.
    const said = screen.getAllByRole("status").filter((status) => status.contains(screen.getByRole("alert")));
    expect(said.map((status) => create().form?.contains(status))).toEqual([true]);
    expect(people().value).toBe("3");
    expect(lives().chosen).toEqual(["1 day"]);
    expect(sections()).toEqual(["New invite"]);

    await user.click(create());

    await made();
    expect(sent.seen.map((seen) => seen.body)).toEqual([
      { max_uses: 3, expires_in: day },
      { max_uses: 3, expires_in: day },
    ]);
  });

  for (const [field, body, sentence] of [
    [
      "max_uses",
      { max_uses: 11 },
      "This server no longer allows that many people for one invite. Enter a smaller number and create the invite again.",
    ],
    [
      "expires_in",
      { expires_in: 31 * day },
      "This server no longer allows an invite to work that long. Choose a shorter time and create the invite again.",
    ],
  ] as const)
    test(`a value the server no longer allows (${field}), the form says which and what to do, and stays`, async () => {
      const problem = await refusalOn({}, body);
      expect([problem.status, problem.field]).toEqual([422, field]);
      const { fake, user } = await createForm();
      fake.script("POST /invites", { fail: problem, times: 1 });

      await user.click(create());

      expect(await refused()).toBe(sentence);
      expect(controls()).toEqual(["number", "choice", "submit"]);
    });

  test("an account that may no longer create invites, the summary that says why stands in the form's place", async () => {
    const problem = await refusalOn({ access: { invites: "not-allowed", invitesRefusal: "restricted" } });
    expect([problem.status, problem.type]).toEqual([403, "/api/v1/problems/invites-not-allowed"]);
    const { fake, user, data } = await createForm();
    // From the refusal on, the server answers the list as for an account that may not create invites.
    fake.script("POST /invites", { fail: problem });
    fake.script("GET /invites", {
      change: (body) => ({ ...body, can_create: false, cannot_create_reason: "restricted" as const }),
    });

    await user.click(create());

    await screen.findByRole("region", { name: cannot });
    await atRest({ data });
    expect(sections()).toEqual([cannot]);
    expect(words(section(cannot))).toMatch(
      inOrder(cannot, "A moderation action keeps your account from creating invites for now."),
    );
    expect(controls()).toEqual([]);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });
});

describe("in the form's place", () => {
  test("an account that may not create invites reads the summary that says why, and no form", async () => {
    const { data } = await createDrawn({ invites: "not-allowed" });

    await screen.findByRole("region", { name: cannot });
    await atRest({ data });
    expect(headings()).toEqual(["Create invite", cannot]);
    expect(words(section(cannot))).toMatch(
      inOrder(
        cannot,
        "Only some accounts can create invites on this server. Ask its administrators to let you create them.",
      ),
    );
    expect(controls()).toEqual([]);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("an account with as many usable invites as the server allows reads so and what to do, and no form", async () => {
    const { data } = await createDrawn({ invites: "at-cap" });

    const limit = await screen.findByText(
      "You have as many usable invites as allowed (5). Revoke one, or wait until one expires or is used up.",
    );
    await atRest({ data });
    expect(headings()).toEqual(["Create invite", "New invite"]);
    expect(section("New invite").contains(limit)).toBe(true);
    expect(controls()).toEqual([]);
    expect(settingsFindings(document.body)).toEqual([]);
  });
});

describe("until the server's limits are read", () => {
  test("the screen says that it loads the settings, and shows the form once the server answered", async () => {
    let held: { release: () => void } | undefined;
    await createDrawn(
      {},
      {
        before: ({ fake }) => {
          held = fake.script("GET /invites", { hold: "request" });
        },
      },
    );
    expect((await screen.findByRole("status")).textContent).toBe(loading);
    expect(controls()).toEqual([]);

    held?.release();

    await waitFor(() => {
      if (controls().length !== 3) throw new Error("test: the form is not drawn yet");
    });
  });

  test("the screen says that the settings could not be loaded when the read fails, and Try again shows the form", async () => {
    const { user } = await createDrawn(
      {},
      {
        before: ({ fake }) => {
          fake.script("GET /invites", { fail: 500, times: 1 });
        },
      },
    );

    const failed = await screen.findByRole("alert");
    expect(words(failed)).toMatch(notLoaded);
    expect(controls()).toEqual([]);

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await waitFor(() => {
      if (controls().length !== 3) throw new Error("test: the form is not drawn yet");
    });
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });
});
