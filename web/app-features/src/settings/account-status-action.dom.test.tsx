// SPDX-License-Identifier: AGPL-3.0-only

// One moderation action on the reader's account and its appeal (#3204, FR-715, ADR-0058 §9, §10), the screen a row of
// Account status leads to: what the action does in the server's words, when it ends, the moderators' reason, the
// posts it is about, and nothing the moderators keep to themselves; and the appeal. An action that can be appealed
// has a form: one field, sent once with Send appeal, which says what is wrong with the field before anything is sent,
// what the server refused, and that the appeal was sent. An appeal made shows where it stands, what the reader wrote
// and what the moderators answered. The form tells its owner whether an appeal is written and not sent, and asks
// before it is left when its owner says the reader is leaving. Where the focus goes is
// account-status-action.jsdom.test.tsx; the lists and the entry, account-status.dom.test.tsx; the routes and the held
// navigation, the app's (web/app/src/settings-account-status.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { appealLimits } from "@my/app-data";
import { screen, waitFor, within } from "@testing-library/react";
import {
  acted,
  alerts,
  answered,
  appealField,
  appealFields,
  describedBy,
  drawnAction,
  interactionUpheld,
  limitWaiting,
  linesOf,
  main,
  moderatorsOnly,
  noPosting,
  postsOverturned,
  said,
  screenWords,
  section,
  sectionsTitled,
  sendButton,
  sensitivePassed,
  serverOf,
  warning,
  warnings,
  withTheModerators,
  wordsOf,
  write,
  type Drawn,
  type Seed,
} from "../test/account-status";

const route = "POST /moderation/actions/{id}/appeal";
const statusPath = "/settings/account/status";
const what = "What this action does";
const reason = "The moderators' reason";
const posts = "Posts this action is about";
const appeal = "Appeal";
const hint = "Say why the moderators should reconsider this action, in up to 2,000 characters.";
const until =
  "You can appeal this action once, until Mar 9, 2031, and you cannot change your appeal after you send it. A moderator reads it and decides.";
const notSent = "The appeal was not sent. Check the field marked above.";
const sent = "Your appeal was sent.";
const waiting = "Your appeal is waiting for a moderator's review. You will be notified of the result.";

/**
 * The lines of the section titled `name`, each moment as its day: the screen writes the hour too, in the reader's
 * zone, which is the suite's here.
 */
const lines = (name: string) =>
  linesOf(section(name)).map((line) => line.replace(/(\d{4}), \d{1,2}:\d{2} [AP]M/g, "$1"));

/** The writes the screen sent, each as `METHOD /path`. */
const writes = ({ requests }: Drawn) => requests.filter((request) => !request.startsWith("GET "));

/** The buttons named Send appeal the screen shows: one while the action can be appealed, none after. */
const sendButtons = () => main().queryAllByRole("button", { name: "Send appeal" });

/** A problem of the server of `type`, as it refuses an appeal. */
const refusal = (type: string, status = 422) =>
  ({ type: `/api/v1/problems/${type}`, title: "This could not be done.", status }) as const;

/** The action drawn once the server answered. */
async function action(...given: Parameters<typeof drawnAction>): Promise<Drawn> {
  const on = await drawnAction(...given);
  await answered(on);
  return on;
}

/** An action in effect that can be appealed with no last day. */
const { appeal: _waiting, appeal_deadline: _day, ...limit } = limitWaiting;
const openEnded: Seed = { ...limit, appealable: true };

describe("one action", () => {
  test("is titled Moderation action and says what it does, until when, since when, and why, with the way back to Account status", async () => {
    const on = await action(acted(noPosting));

    expect(main().getAllByRole("heading", { name: "Moderation action" })).toHaveLength(1);
    expect(lines(what)).toEqual([
      "No posting, replying, boosting, or editing",
      "Ends on May 20, 2031",
      "Taken on Nov 2, 2024",
    ]);
    expect(lines(reason)).toEqual(["Spam", "The same link was posted to forty hashtags within an hour."]);
    // An action on the whole account is about no post.
    expect(sectionsTitled(posts)).toEqual([]);

    await on.user.click(main().getByRole("button", { name: "Account status" }));

    expect(on.told.filter((word) => word === "back")).toEqual(["back"]);
  });

  const ends: readonly [string, Seed, string][] = [
    ["an action with no end", limitWaiting, "Lasts until a moderator lifts it"],
    ["an action that ended", interactionUpheld, "Ended on Feb 15, 2024"],
    ["an action a moderator lifted", postsOverturned, "Lifted on Jan 18, 2024"],
  ];
  for (const [which, seed, line] of ends)
    test(`${which} says "${line}"`, async () => {
      await action(acted(seed));

      expect(lines(what)[1]).toBe(line);
    });

  test("what it does and the kind of reason are the server's own words, whatever they are", async () => {
    await action(
      acted({ ...noPosting, kind_label: "Posting is paused", reason_category_label: "Unwanted advertising" }),
    );

    expect(lines(what)[0]).toBe("Posting is paused");
    expect(lines(reason)[0]).toBe("Unwanted advertising");
  });

  test("an action the moderators did not explain says so under its kind of reason", async () => {
    await action(acted(warning));

    expect(lines(reason)).toEqual([
      "Violation of the server's rules",
      "The moderators gave no explanation with this action.",
    ]);
  });

  test("an action on posts links the ones that still open, and counts the ones that are gone", async () => {
    await action(acted({ ...postsOverturned, ownPosts: 2, status_ids: ["01HZZZZZZZZZZZZZZZZZZZZZZZ"] }));

    const list = await within(section(posts)).findByRole("list", { name: posts });
    const links = within(list).getAllByRole("link");
    expect(links).toHaveLength(2);
    expect(new Set(links.map((link) => link.getAttribute("href"))).size).toBe(2);
    for (const link of links) {
      expect(link.getAttribute("href")).toMatch(/^\/@yuki\/status\/[^/]+$/);
      expect(wordsOf(link)).toMatch(/^Post of \w{3} \d{1,2}, \d{4}/);
    }
    expect(lines(posts)).toEqual(["1 post is no longer on this server, so it cannot be opened."]);
  });

  test("an action whose posts are all gone says how many, and links none", async () => {
    await action(
      acted({ ...postsOverturned, status_ids: ["01HZZZZZZZZZZZZZZZZZZZZZZY", "01HZZZZZZZZZZZZZZZZZZZZZZZ"] }),
    );

    await waitFor(() => {
      expect(lines(posts)).toEqual(["2 posts are no longer on this server, so they cannot be opened."]);
    });
    expect(within(section(posts)).queryAllByRole("link")).toHaveLength(0);
  });

  test("shows nothing a moderator keeps to the moderators, should a server ever send it", async () => {
    const server = serverOf(acted(interactionUpheld));
    const leaking = server.fake.script("GET /moderation/actions", { change: withTheModerators });
    await action(server);

    expect(leaking.seen.length).toBeGreaterThan(0);
    // The screen is drawn whole: its last section holds the moderators' answer.
    expect(lines(appeal).at(-1)).toBe(
      "The follows continued after the first warning, so the restriction stays until it ends.",
    );
    expect(screenWords()).not.toMatch(moderatorsOnly);
  });

  test("an action past the first page of the reader's actions is found: the list is read on until it holds it", async () => {
    const on = await action(acted(...warnings(24)), { index: 22 });

    await waitFor(() => {
      expect(lines(reason)).toEqual(["Violation of the server's rules", "Warning number 23."]);
    });
    expect(on.requests.filter((request) => request.startsWith("GET /moderation/actions"))).toHaveLength(2);
  });

  test("an address that names no action of the reader's says so in an alert, with the way to Account status", async () => {
    await action(acted(noPosting), { id: "nothing" });

    expect(alerts()).toHaveLength(1);
    expect(alerts()[0]).toMatch(
      /^This moderation action is not listed on your account\. Check the address\.\s*Account status$/,
    );
    expect(main().getByRole("link", { name: "Account status" }).getAttribute("href")).toBe(statusPath);
    expect(main().queryAllByRole("region")).toHaveLength(0);
    expect(appealFields()).toHaveLength(0);
  });

  test("when the actions could not be read the screen says so in an alert, and Try again reads them", async () => {
    const server = serverOf(acted(noPosting));
    const failing = server.fake.script("GET /moderation/actions", { fail: 500 });
    const on = await drawnAction(server);

    const again = await main().findByRole("button", { name: "Try again" });
    expect(alerts()).toHaveLength(1);
    expect(alerts()[0]).toMatch(/^Your account status could not be loaded\.\s*Try again$/);
    expect(main().queryAllByRole("region")).toHaveLength(0);
    failing.end();
    const before = on.requests.length;

    await on.user.click(again);

    await answered(on, before);
    expect(lines(what)[0]).toBe("No posting, replying, boosting, or editing");
    expect(alerts()).toEqual([]);
  });
});

describe("where an appeal stands", () => {
  test("an appeal that waits: so it says, with when it was sent and what the reader wrote", async () => {
    await action(acted(limitWaiting));

    expect(lines(appeal)).toEqual([
      waiting,
      "Sent on Oct 21, 2024",
      "Your appeal",
      "I replied to three threads that day, each on its subject. Please look at the replies again.",
    ]);
    expect(appealFields()).toHaveLength(0);
    expect(sendButtons()).toHaveLength(0);
    expect(said()).toEqual([]);
  });

  test("an appeal the moderators decided by keeping the action: so it says, with when it was reviewed and their answer", async () => {
    await action(acted(interactionUpheld));

    expect(lines(appeal)).toEqual([
      "A moderator reviewed your appeal and kept the action.",
      "Sent on Feb 2, 2024",
      "Reviewed on Feb 4, 2024",
      "Your appeal",
      "I was looking for people who post about baking and went too fast. I have stopped.",
      "The moderators' answer",
      "The follows continued after the first warning, so the restriction stays until it ends.",
    ]);
    expect(appealFields()).toHaveLength(0);
    expect(sendButtons()).toHaveLength(0);
  });

  test("an appeal the moderators decided by lifting the action: so it says", async () => {
    await action(acted(postsOverturned));

    expect(lines(appeal)).toEqual([
      "A moderator reviewed your appeal and lifted the action.",
      "Sent on Jan 12, 2024",
      "Reviewed on Jan 18, 2024",
      "Your appeal",
      "The photos are of my own recovery and I added a warning to each.",
      "The moderators' answer",
      "On a second look the posts carried the warning. The action is lifted.",
    ]);
  });

  test("an appeal decided with no answer of the moderators shows none", async () => {
    const { decision_reason: _answer, ...decided } = interactionUpheld.appeal ?? {};
    await action(acted({ ...interactionUpheld, appeal: decided as NonNullable<Seed["appeal"]> }));

    expect(lines(appeal)).toEqual([
      "A moderator reviewed your appeal and kept the action.",
      "Sent on Feb 2, 2024",
      "Reviewed on Feb 4, 2024",
      "Your appeal",
      "I was looking for people who post about baking and went too fast. I have stopped.",
    ]);
  });

  test("an action never appealed whose last day passed says when, and has no form", async () => {
    await action(acted(sensitivePassed));

    expect(lines(appeal)).toEqual(["You did not appeal this action, and the time to appeal passed on Jun 1, 2024."]);
    expect(appealFields()).toHaveLength(0);
    expect(sendButtons()).toHaveLength(0);
  });
});

describe("the appeal form", () => {
  test("an action that can be appealed: until when and that it is sent once, one field with what to write in it, and Send appeal", async () => {
    await action(acted(noPosting));

    expect(lines(appeal)[0]).toBe(until);
    expect(appealFields()).toHaveLength(1);
    expect(appealField().value).toBe("");
    expect(describedBy(appealField())).toEqual([hint]);
    expect(appealField().getAttribute("aria-invalid")).not.toBe("true");
    expect(sendButton().disabled).toBe(false);
    expect(said()).toEqual([]);
    expect(alerts()).toEqual([]);
  });

  test("an action that can be appealed with no last day names none", async () => {
    await action(acted(openEnded));

    expect(lines(appeal)[0]).toBe(
      "You can appeal this action once, and you cannot change your appeal after you send it. A moderator reads it and decides.",
    );
    expect(appealFields()).toHaveLength(1);
  });

  for (const [what_, written] of [
    ["nothing written", ""],
    ["blanks alone", "  \n "],
  ] as const)
    test(`Send appeal with ${what_} sends nothing: the field stands invalid, described by what to do, and the bar says the appeal was not sent`, async () => {
      const on = await action(acted(noPosting));
      if (written !== "") await write(on.user, written);

      await on.user.click(sendButton());

      await waitFor(() => {
        expect(alerts()).toEqual([notSent]);
      });
      expect(appealField().getAttribute("aria-invalid")).toBe("true");
      expect(describedBy(appealField())).toEqual([hint, "Write your appeal before you send it."]);
      expect(writes(on)).toEqual([]);
      expect(said()).not.toContain(sent);
    });

  test("Send appeal with more than 2,000 characters sends nothing, and the field says the most it takes", async () => {
    const on = await action(acted(noPosting));
    await write(on.user, "a".repeat(appealLimits.text + 1));

    await on.user.click(sendButton());

    await waitFor(() => {
      expect(alerts()).toEqual([notSent]);
    });
    expect(appealField().getAttribute("aria-invalid")).toBe("true");
    expect(describedBy(appealField())).toEqual([hint, "Your appeal must be at most 2,000 characters."]);
    expect(writes(on)).toEqual([]);
    // What the reader wrote is kept for them to shorten.
    expect(appealField().value).toHaveLength(appealLimits.text + 1);
  });

  test("an appeal of exactly 2,000 characters is sent whole", async () => {
    const on = await action(acted(noPosting));
    const seen = on.fake.script(route);
    await write(on.user, "a".repeat(appealLimits.text));

    await on.user.click(sendButton());

    await waitFor(() => {
      expect(said()).toEqual([sent]);
    });
    expect(seen.seen.map((request) => request.body.text.length)).toEqual([appealLimits.text]);
  });

  test("what the field said is no longer said once the reader writes in it", async () => {
    const on = await action(acted(noPosting));
    await on.user.click(sendButton());
    await waitFor(() => {
      expect(alerts()).toEqual([notSent]);
    });

    await write(on.user, "Please look again.");

    await waitFor(() => {
      expect(appealField().getAttribute("aria-invalid")).not.toBe("true");
    });
    expect(describedBy(appealField())).toEqual([hint]);
    expect(alerts()).toEqual([]);
  });

  test("Send appeal sends what was written, without the blanks around it, once: the form gives way to a status that says it was sent, over where the appeal stands", async () => {
    const on = await action(acted(noPosting));
    const seen = on.fake.script(route);
    await write(on.user, "  Please look again.\nI posted the link once.  ");

    await on.user.click(sendButton());

    await waitFor(() => {
      expect(said()).toEqual([sent]);
    });
    expect(seen.seen.map((request) => request.body)).toEqual([{ text: "Please look again.\nI posted the link once." }]);
    expect(writes(on)).toEqual([`POST /moderation/actions/${on.fake.actionPath(0).split("/").at(-1) ?? ""}/appeal`]);
    expect(appealFields()).toHaveLength(0);
    expect(sendButtons()).toHaveLength(0);
    expect(alerts()).toEqual([]);
    // Where the appeal stands, in the form's place: no last day to appeal is left to say.
    const [stands, sentOn, ...written] = lines(appeal);
    expect(stands).toBe(waiting);
    expect(sentOn).toMatch(/^Sent on \w{3} \d{1,2}, \d{4}$/);
    expect(written).toEqual(["Your appeal", "Please look again. I posted the link once."]);
  });

  test("while the server has not answered, Send appeal is busy, the field cannot be changed, and a second press sends nothing more", async () => {
    const on = await action({ settings: { moderation: [noPosting], appealSave: "waits" } });
    const seen = on.fake.script(route);
    await write(on.user, "Please look again.");

    await on.user.click(sendButton());

    await waitFor(() => {
      expect(sendButton().getAttribute("aria-busy")).toBe("true");
    });
    expect(appealField().readOnly || appealField().disabled).toBe(true);
    expect(appealField().value).toBe("Please look again.");
    await on.user.click(sendButton());
    expect(seen.seen).toHaveLength(1);
    expect(said()).toEqual([]);
  });

  for (const fail of [500, "network"] as const)
    test(`an appeal the server did not take (${String(fail)}): the bar says it could not be sent, the form keeps what was written, and the next Send appeal sends it`, async () => {
      const on = await action(acted(noPosting));
      const seen = on.fake.script(route);
      const failing = on.fake.script(route, { times: 1, fail });
      await write(on.user, "Please look again.");

      await on.user.click(sendButton());

      await waitFor(() => {
        expect(alerts()).toEqual(["The appeal could not be sent. Check your connection and try again."]);
      });
      expect(appealField().value).toBe("Please look again.");
      expect(appealField().getAttribute("aria-invalid")).not.toBe("true");
      expect(lines(appeal)[0]).toBe(until);
      expect(said()).not.toContain(sent);
      failing.end();

      await on.user.click(sendButton());

      await waitFor(() => {
        expect(said()).toEqual([sent]);
      });
      expect(alerts()).toEqual([]);
      expect(seen.seen.map((request) => request.body)).toEqual([
        { text: "Please look again." },
        { text: "Please look again." },
      ]);
    });

  test("in Japanese the form and what it says once sent read in it", async () => {
    const on = await action(acted(noPosting), { locale: "ja" });
    const field = main().getByRole("textbox", { name: "異議申し立ての内容" });
    await on.user.click(field);
    await on.user.paste("もう一度確認してください。");

    await on.user.click(main().getByRole("button", { name: "異議を申し立てる" }));

    await waitFor(() => {
      expect(said()).toEqual(["異議申し立てを送りました。"]);
    });
  });
});

describe("an appeal the server refuses for good", () => {
  const about = "this server's About page";
  // Why the server refused, how a test makes it refuse so, what the status says, and whether it leads to the About page.
  const refusals: readonly {
    why: string;
    on: () => ReturnType<typeof serverOf>;
    says: string;
    leads: boolean;
  }[] = [
    {
      why: "the time to appeal has passed",
      on: () => serverOf({ settings: { moderation: [noPosting], appealSave: "closed" } }),
      says: "The time to appeal this action has passed, so your appeal was not sent. To reach the moderators, see the contact on this server's About page.",
      leads: true,
    },
    {
      why: "the action can no longer be appealed",
      on: () => {
        const server = serverOf(acted(noPosting));
        server.fake.script(route, { fail: refusal("not-appealable") });
        return server;
      },
      says: "This action can no longer be appealed, so your appeal was not sent. To reach the moderators, see the contact on this server's About page.",
      leads: true,
    },
    {
      why: "the action was appealed already",
      on: () => {
        const server = serverOf(acted(noPosting));
        server.fake.script(route, { fail: refusal("already-appealed") });
        return server;
      },
      says: "You have already appealed this action, and an action can be appealed once. Your earlier appeal and where it stands are shown here.",
      leads: false,
    },
    {
      why: "the action is no longer the reader's",
      on: () => {
        const server = serverOf(acted(noPosting));
        server.fake.script(route, { fail: 404 });
        return server;
      },
      says: "This action is no longer listed on your account, so your appeal was not sent. Go back to “Account status” to see the actions on your account.",
      leads: false,
    },
  ];

  for (const { why, on: made, says, leads } of refusals)
    test(`${why}: the form is gone, a status says why, and what the reader wrote stays to be copied`, async () => {
      const on = await action(made());
      await write(on.user, "Please look again.");

      await on.user.click(sendButton());

      await waitFor(() => {
        expect(said()).toEqual([says]);
      });
      expect(appealFields()).toHaveLength(0);
      expect(sendButtons()).toHaveLength(0);
      expect(alerts()).toEqual([]);
      // No last day to appeal is said any more.
      expect(lines(appeal)).not.toContain(until);
      expect(lines(appeal).slice(-3)).toEqual([
        "What you wrote, not sent",
        "Please look again.",
        "Copy it if you want to send it to the moderators another way. It is not kept after you leave this page.",
      ]);
      expect(
        main()
          .queryAllByRole("link", { name: about })
          .map((link) => link.getAttribute("href")),
      ).toEqual(leads ? ["/about"] : []);
    });

  test("an action appealed elsewhere meanwhile: the status says so, over the earlier appeal as the server holds it", async () => {
    const on = await action(acted(noPosting));
    // The server takes the appeal, and answers as it would a second one.
    const taken = on.fake.script(route, { hold: "response", fail: refusal("already-appealed") });
    await write(on.user, "Please look again.");

    await on.user.click(sendButton());
    await taken.served();
    taken.release();

    await waitFor(() => {
      expect(said()).toEqual([
        "You have already appealed this action, and an action can be appealed once. Your earlier appeal and where it stands are shown here.",
      ]);
      expect(lines(appeal)).toContain(waiting);
    });
    expect(appealFields()).toHaveLength(0);
  });
});

describe("the form tells its owner whether an appeal is written and not sent", () => {
  /** The last the owner was told of it. */
  const unsaved = ({ told }: Drawn) => told.findLast((word) => word.startsWith("unsaved"));

  test("none when it opens, one once something is written, and none again once it is sent", async () => {
    const on = await action(acted(noPosting));
    expect(unsaved(on)).not.toBe("unsaved:true");

    await write(on.user, "Please look again.");
    await waitFor(() => {
      expect(unsaved(on)).toBe("unsaved:true");
    });

    await on.user.click(sendButton());
    await waitFor(() => {
      expect(said()).toEqual([sent]);
    });
    await waitFor(() => {
      expect(unsaved(on)).toBe("unsaved:false");
    });
  });

  test("none once what was written is taken out again", async () => {
    const on = await action(acted(noPosting));
    await write(on.user, "a");
    await waitFor(() => {
      expect(unsaved(on)).toBe("unsaved:true");
    });

    await on.user.clear(appealField());

    await waitFor(() => {
      expect(unsaved(on)).toBe("unsaved:false");
    });
  });

  test("an appeal the server did not take stays written and not sent", async () => {
    const on = await action(acted(noPosting));
    on.fake.script(route, { fail: 500 });
    await write(on.user, "Please look again.");

    await on.user.click(sendButton());
    await waitFor(() => {
      expect(alerts()).toHaveLength(1);
    });

    expect(unsaved(on)).toBe("unsaved:true");
  });
});

describe("leaving with an appeal written and not sent", () => {
  /** The confirmation the screen shows while its owner says the reader is leaving. */
  const asking = () => screen.getByRole("alertdialog", { name: "Leave without sending your appeal?" });

  test("the screen asks first, in an alert dialog that says what is lost, with Keep writing and Leave", async () => {
    await action(acted(noPosting), { leaving: true });

    expect(describedBy(asking())).toEqual(["What you wrote will be lost. This cannot be undone."]);
    expect(
      within(asking())
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["Close", "Keep writing", "Leave"]);
  });

  for (const [button, tells] of [
    ["Keep writing", "stay"],
    ["Close", "stay"],
    ["Leave", "leave"],
  ] as const)
    test(`${button} tells the owner to ${tells}`, async () => {
      const on = await action(acted(noPosting), { leaving: true });

      await on.user.click(within(asking()).getByRole("button", { name: button }));

      expect(on.told.filter((word) => word === "stay" || word === "leave")).toEqual([tells]);
    });

  test("no confirmation shows while the reader is not leaving", async () => {
    const on = await action(acted(noPosting));
    await write(on.user, "Please look again.");

    expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  });
});
