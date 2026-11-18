// SPDX-License-Identifier: AGPL-3.0-only

// Profile, the settings' entry (#3200): the reader's profile as others see it with the way to edit it, and the choices
// of how the account is found, each of which shows what the server holds and saves a change at once with a request of
// its own (the featured hashtags as a whole list, a badge by its role, everything else as one member of the account's
// credentials), saying in its row how the save went. Where the server decides for every account, the screen says what
// the reader's choice still does, or that it does nothing. Before the badges and the featured hashtags are read the
// screen says that it loads them, and when they cannot be read, that too. The form the entry leads to is
// profile-edit.dom.test.tsx; the entry's place in the settings, and a save that fails after the reader left, are the
// app's (web/app/src/settings-profile.dom.test.tsx); how long "Saved" stays is Display and posting's
// (display.dom.test.tsx), which draws the same rows.

import { describe, expect, onTestFinished, test } from "bun:test";
import { badgeChoicesQuery, instanceQuery, updateProfile, viewerQuery, type ProfileChange } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { fakeData, renderFeature, type FakeData } from "../test/feature";
import { SettingsEntryScreen } from "./screen";

/**
 * Draws Profile on the server `on`, in the main landmark as the shell places it, once the shell read the instance and
 * the reader's account, and returns the reader's hands. The server's answers reach the screen when they come, not
 * inside an act() of the test, so React is told so until the test ends, and a test waits for what it asserts.
 */
async function drawn({ data }: FakeData): Promise<UserEvent> {
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  await data.queryClient.query(instanceQuery(data));
  await data.queryClient.query(viewerQuery(data));
  return renderFeature(
    data,
    <main>
      <SettingsEntryScreen entry="profile" onBack={() => undefined} languages={["en", "ja"]} />
    </main>,
  );
}

const hashtags = "Featured hashtags";
const bot = "Mark this account as a bot";
const locked = "Require approval to follow you";
const lists = "Who can see your following and followers lists";
const discoverable = "Appear in suggestions and in search by name";
const indexable = "Include your profile and public posts in search";
const ai = "Refuse AI training and use";
const feed = "Offer an Atom feed of your public posts";

/**
 * Profile on a server of `options`, with its controls drawn: the badges and the featured hashtags are read. `own` is
 * what the account chose before, which the server keeps as it keeps any change.
 */
async function read(options: FakeOptions = {}, own: ProfileChange = {}) {
  const on = fakeData(options);
  if (Object.keys(own).length > 0) await updateProfile(on.data, own);
  const user = await drawn(on);
  await screen.findByRole("textbox", { name: hashtags });
  return { ...on, user };
}

/** The switch named `name`. */
const toggle = (name: string) => screen.getByRole<HTMLInputElement>("switch", { name });
/** The name of each of `switches`, in their order: the one of `names` that finds it; empty for a switch none finds. */
const named = (switches: readonly HTMLElement[], names: readonly string[]) =>
  switches.map((control) => names.find((name) => screen.queryByRole("switch", { name }) === control) ?? "");
/** The choice named `name`. */
const choice = (name: string) => screen.getByRole<HTMLSelectElement>("combobox", { name });
/** What the choice named `name` reads: the words of the option it is at. */
const chosen = (name: string) => choice(name).selectedOptions[0]?.textContent;
/** The field that adds a featured hashtag. */
const tagField = () => screen.getByRole<HTMLInputElement>("textbox", { name: hashtags });
/** The hashtags featured, as the buttons that remove them name them. */
const featured = () =>
  screen
    .queryAllByRole("button", { name: /^Remove #/ })
    .map((remove) => (remove.getAttribute("aria-label") ?? "").replace("Remove #", ""));
/** The section of the screen titled `name`. */
const section = (name: string) => within(screen.getByRole("region", { name }));
/** The reader's profile as others see it: the group their name names. */
const profile = (name = "Yuki 雪") => screen.getByRole("group", { name });
/**
 * What the status regions of the screen say: each row that saves at once keeps one from the start, empty while it has
 * nothing to say, so a state is found by its words.
 */
const said = () =>
  screen
    .queryAllByRole("status")
    .map((status) => status.textContent)
    .filter((words) => words !== "");
/** The line about `control`: what describes it, without what its row says of a save. */
const about = (control: HTMLElement) =>
  (control.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id))
    .filter((part) => part !== null && part.getAttribute("role") !== "status")
    .map((part) => part?.textContent ?? "");

const flip = (name: string) => (user: UserEvent) => user.click(toggle(name));
const pick = (name: string, option: string) => (user: UserEvent) => user.selectOptions(choice(name), option);
const press = (name: string) => (user: UserEvent) => user.click(screen.getByRole("button", { name }));
/** Writes `text` in the field of the featured hashtags, a key at a time: a key may be what adds the hashtag. */
const write = (text: string) => async (user: UserEvent) => {
  await user.click(tagField());
  await user.keyboard(text);
};

describe("the reader's profile, as others see it", () => {
  test("is a group named by the account's name: its pictures, its name, its handle, the badges it shows and its bio, with the way to Edit profile", async () => {
    await read();

    const own = within(section("Your profile").getByRole("group", { name: "Yuki 雪" }));
    expect(own.getByText("@yuki@demo.test")).not.toBeNull();
    expect(own.getByText("写真と散歩。週末は山にいます。")).not.toBeNull();
    // The badge the account shows, and not the one it holds without showing.
    expect(own.getByText("Moderator")).not.toBeNull();
    expect(own.queryAllByText("Supporter")).toHaveLength(0);
    // The header and the avatar, both decorations: the name beside them says whose they are.
    expect(own.getAllByRole("presentation", { hidden: true })).toHaveLength(2);
    expect(own.queryAllByRole("img")).toHaveLength(0);
    expect(own.queryAllByText("Bot")).toHaveLength(0);
    expect(own.getAllByRole("link").map((link) => [link.textContent, link.getAttribute("href")])).toEqual([
      ["Edit profile", "/settings/profile/edit"],
    ]);
  });

  test("says Bot beside the name of an account marked as one, and that a locked account approves its followers", async () => {
    await read({}, { bot: true, locked: true });

    const own = within(profile());
    expect(own.getAllByText("Bot")).toHaveLength(1);
    expect(own.getAllByRole("img").map((sign) => sign.getAttribute("aria-label"))).toEqual([
      "Follow requests need approval",
    ]);
  });

  test("of an account with no name, picture or bio is named by its username, with no picture", async () => {
    await read({ profile: { start: "empty" } });

    const own = within(section("Your profile").getByRole("group", { name: "yuki" }));
    expect(own.getByText("@yuki@demo.test")).not.toBeNull();
    expect(own.queryAllByRole("presentation", { hidden: true })).toHaveLength(0);
    expect(own.getAllByRole("link", { name: "Edit profile" })).toHaveLength(1);
  });
});

describe("each control is at what the server holds", () => {
  test("the scenario's account: two hashtags, a badge shown and one not, found everywhere, its lists open to anyone", async () => {
    await read();

    expect(featured()).toEqual(["baking", "photography"]);
    expect(tagField().value).toBe("");
    expect(toggle(bot).checked).toBe(false);
    expect(toggle("Show “Moderator”").checked).toBe(true);
    expect(toggle("Show “Supporter”").checked).toBe(false);
    expect(toggle(locked).checked).toBe(false);
    expect(chosen(lists)).toBe("Anyone");
    expect(toggle(discoverable).checked).toBe(true);
    expect(toggle(indexable).checked).toBe(true);
    expect(toggle(ai).checked).toBe(false);
    expect(toggle(feed).checked).toBe(true);
    // No row has anything to say of a save yet.
    expect(said()).toEqual([]);
  });

  test("an account's own choices", async () => {
    await read(
      { settings: { featured: [], badges: [{ label: "Translator", color: "neutral", displayed: false }] } },
      {
        bot: true,
        locked: true,
        follows_visibility: "followers",
        discoverable: false,
        indexable: false,
        ai_opt_out: true,
        feed_enabled: true,
      },
    );

    expect(featured()).toEqual([]);
    expect(toggle(bot).checked).toBe(true);
    expect(toggle("Show “Translator”").checked).toBe(false);
    expect(toggle(locked).checked).toBe(true);
    expect(chosen(lists)).toBe("Followers");
    expect(toggle(discoverable).checked).toBe(false);
    expect(toggle(indexable).checked).toBe(false);
    expect(toggle(ai).checked).toBe(true);
    expect(toggle(feed).checked).toBe(true);
  });

  test("the sections stand in their order, the profile's own choices with the profile", async () => {
    await read();

    expect(screen.getAllByRole("region").map((part) => within(part).getAllByRole("heading")[0]?.textContent)).toEqual([
      "Your profile",
      "Badges",
      "Following and followers",
      "How your account is found",
    ]);
    expect(section("Your profile").getAllByRole("textbox", { name: hashtags })).toHaveLength(1);
    expect(section("Your profile").getAllByRole("switch", { name: bot })).toHaveLength(1);
    expect(section("Following and followers").getAllByRole("switch", { name: locked })).toHaveLength(1);
    expect(section("Following and followers").getAllByRole("combobox", { name: lists })).toHaveLength(1);
    expect(
      named(section("How your account is found").getAllByRole("switch"), [feed, ai, indexable, discoverable]),
    ).toEqual([discoverable, indexable, ai, feed]);
  });

  test("who can see the lists of follows offers anyone, the followers, or the account alone", async () => {
    await read();

    expect(
      within(choice(lists))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["Anyone", "Followers", "Only you"]);
  });

  test("each choice says what it does", async () => {
    await read();

    expect(about(tagField())).toEqual(["Add up to 10. They show on your profile."]);
    expect(about(toggle(bot))).toEqual(["For an account that a program posts from. “Bot” shows beside your name."]);
    expect(about(toggle(locked))).toEqual([
      "People ask to follow you, and you approve or decline each request. Turning this off does not approve requests that are already waiting.",
    ]);
    expect(about(choice(lists))).toEqual(["The numbers stay visible to anyone."]);
    expect(about(toggle(discoverable))).toEqual([
      "When off, the search finds your account only by its whole username.",
    ]);
  });
});

/** A change of the account's credentials a reader makes on the screen: where it starts, what they do, what is sent. */
type Change = {
  what: string;
  from?: ProfileChange;
  act: (user: UserEvent) => Promise<void>;
  sends: ProfileChange;
};

const changes: readonly Change[] = [
  { what: "marking the account as a bot", act: flip(bot), sends: { bot: true } },
  { what: "a bot's account no longer one", from: { bot: true }, act: flip(bot), sends: { bot: false } },
  { what: "approving followers", act: flip(locked), sends: { locked: true } },
  { what: "the lists for followers", act: pick(lists, "Followers"), sends: { follows_visibility: "followers" } },
  { what: "the lists for the account alone", act: pick(lists, "Only you"), sends: { follows_visibility: "private" } },
  {
    what: "the lists for anyone again",
    from: { follows_visibility: "private" },
    act: pick(lists, "Anyone"),
    sends: { follows_visibility: "public" },
  },
  {
    what: "staying out of suggestions and the search by name",
    act: flip(discoverable),
    sends: { discoverable: false },
  },
  { what: "staying out of search", act: flip(indexable), sends: { indexable: false } },
  { what: "refusing AI", act: flip(ai), sends: { ai_opt_out: true } },
  { what: "offering no Atom feed", act: flip(feed), sends: { feed_enabled: false } },
];

describe("a choice is saved at once, with a PATCH of that member of the credentials alone", () => {
  for (const { what, from, act: change, sends } of changes)
    test(what, async () => {
      const { fake, user, requests } = await read({}, from);
      const sent = fake.script("PATCH /accounts/update_credentials");
      const before = requests.length;

      await change(user);

      expect((await sent.served()).body).toEqual(sends);
      expect(sent.seen).toHaveLength(1);
      // Nothing else is written: not the hashtags, not a badge.
      expect(requests.slice(before).filter((request) => !request.startsWith("GET "))).toEqual([
        "PATCH /accounts/update_credentials",
      ]);
    });
});

/** A change of the featured hashtags: what the account features before, what the reader does, the list that is sent. */
type TagChange = { what: string; from?: readonly string[]; act: (user: UserEvent) => Promise<void>; sends: string[] };

const tagChanges: readonly TagChange[] = [
  { what: "a hashtag and Enter", act: write("sourdough{Enter}"), sends: ["baking", "photography", "sourdough"] },
  { what: "a hashtag and a comma", act: write("sourdough,"), sends: ["baking", "photography", "sourdough"] },
  { what: "a hashtag and a space", act: write("sourdough "), sends: ["baking", "photography", "sourdough"] },
  {
    what: "a hashtag, and the focus leaving the field",
    act: async (user) => {
      await write("sourdough")(user);
      await user.tab();
    },
    sends: ["baking", "photography", "sourdough"],
  },
  {
    what: "a hashtag written with its #",
    act: write("#Sourdough{Enter}"),
    sends: ["baking", "photography", "Sourdough"],
  },
  { what: "the first hashtag of an account", from: [], act: write("baking{Enter}"), sends: ["baking"] },
  { what: "Remove of a hashtag", act: press("Remove #baking"), sends: ["photography"] },
  { what: "Remove of the last hashtag", from: ["baking"], act: press("Remove #baking"), sends: [] },
];

describe("the featured hashtags are saved at once, with a PUT of the whole list", () => {
  for (const { what, from, act: change, sends } of tagChanges)
    test(what, async () => {
      const { fake, user, requests } = await read(from === undefined ? {} : { settings: { featured: from } });
      const sent = fake.script("PUT /featured_tags");
      const before = requests.length;

      await change(user);

      expect((await sent.served()).body).toEqual({ names: sends });
      await waitFor(() => {
        expect(featured()).toEqual(sends);
      });
      expect(sent.seen).toHaveLength(1);
      expect(requests.slice(before).filter((request) => !request.startsWith("GET "))).toEqual(["PUT /featured_tags"]);
      // The field is empty for the next one.
      expect(tagField().value).toBe("");
    });

  test("a hashtag already featured, in whatever case, and a field left empty send nothing", async () => {
    const { fake, user } = await read();
    const sent = fake.script("PUT /featured_tags");

    await write("Baking{Enter}")(user);
    await write("#PHOTOGRAPHY,")(user);
    await write("{Enter}")(user);
    await user.tab();
    // The next hashtag the reader adds is the first the server hears of.
    await write("sourdough{Enter}")(user);

    expect((await sent.served()).body).toEqual({ names: ["baking", "photography", "sourdough"] });
    expect(sent.seen).toHaveLength(1);
  });

  test("with ten featured the field takes no more and says so, until one is removed", async () => {
    const ten = ["one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"];
    const { fake, user } = await read({ settings: { featured: ten } });
    const sent = fake.script("PUT /featured_tags");

    expect(featured()).toEqual(ten);
    expect(tagField().readOnly).toBe(true);
    expect(tagField().placeholder).toBe("No more hashtags");
    await write("eleven{Enter}")(user);
    expect(tagField().value).toBe("");

    await user.click(screen.getByRole("button", { name: "Remove #ten" }));

    expect((await sent.served()).body).toEqual({ names: ten.slice(0, 9) });
    expect(sent.seen).toHaveLength(1);
    await waitFor(() => {
      expect(tagField().readOnly).toBe(false);
    });
    expect(tagField().placeholder).not.toBe("No more hashtags");
  });

  test("a ninth hashtag leaves room for one more, and the tenth for none", async () => {
    const eight = ["one", "two", "three", "four", "five", "six", "seven", "eight"];
    const { fake, user } = await read({ settings: { featured: eight } });
    const sent = fake.script("PUT /featured_tags");

    await write("nine{Enter}")(user);
    await sent.served();
    await waitFor(() => {
      expect(featured()).toHaveLength(9);
    });
    expect(tagField().readOnly).toBe(false);

    await write("ten{Enter}")(user);
    await sent.served(2);
    await waitFor(() => {
      expect(tagField().readOnly).toBe(true);
    });
    expect(featured()).toEqual([...eight, "nine", "ten"]);
  });
});

describe("the badges", () => {
  test("are a list named Badges with a switch for each badge the server gave the account", async () => {
    await read();

    const badges = section("Badges").getByRole("list", { name: "Badges" });
    expect(
      named(
        within(badges)
          .getAllByRole("listitem")
          .map((item) => within(item).getByRole("switch")),
        ["Show “Supporter”", "Show “Moderator”"],
      ),
    ).toEqual(["Show “Moderator”", "Show “Supporter”"]);
  });

  for (const [label, displayed] of [
    ["Supporter", true],
    ["Moderator", false],
  ] as const)
    test(`${displayed ? "showing" : "hiding"} a badge is saved at once, with a PUT of that role's badge alone`, async () => {
      const { fake, data, user, requests } = await read();
      const held = await data.queryClient.query(badgeChoicesQuery(data));
      const role = held.items.find((badge) => badge.label === label)?.role_id ?? "";
      const sent = fake.script("PUT /preferences/badges/{role_id}");
      const before = requests.length;

      await user.click(toggle(`Show “${label}”`));

      const request = await sent.served();
      expect(request.body).toEqual({ displayed });
      expect(request.url.pathname.endsWith(`/preferences/badges/${role}`)).toBe(true);
      expect(role).not.toBe("");
      expect(sent.seen).toHaveLength(1);
      expect(requests.slice(before).filter((r) => !r.startsWith("GET "))).toEqual([`PUT /preferences/badges/${role}`]);
      expect(toggle(`Show “${label}”`).checked).toBe(displayed);
    });

  test("an account with no badge has no Badges section", async () => {
    await read({ settings: { badges: [] } });

    expect(screen.getAllByRole("region").map((part) => within(part).getAllByRole("heading")[0]?.textContent)).toEqual([
      "Your profile",
      "Following and followers",
      "How your account is found",
    ]);
    expect(screen.queryAllByRole("list", { name: "Badges" })).toHaveLength(0);
    expect(screen.queryAllByRole("switch", { name: /^Show “/ })).toHaveLength(0);
  });
});

describe("the profile above shows a choice once it is saved", () => {
  test("Bot beside the name of an account marked as a bot, and gone when it is one no longer", async () => {
    const { user } = await read();

    await user.click(toggle(bot));
    await waitFor(() => {
      expect(within(profile()).queryAllByText("Bot")).toHaveLength(1);
    });

    await user.click(toggle(bot));
    await waitFor(() => {
      expect(within(profile()).queryAllByText("Bot")).toHaveLength(0);
    });
  });

  test("that follow requests need approval, of an account that approves its followers", async () => {
    const { user } = await read();
    expect(within(profile()).queryAllByRole("img")).toHaveLength(0);

    await user.click(toggle(locked));

    await waitFor(() => {
      expect(
        within(profile())
          .queryAllByRole("img")
          .map((sign) => sign.getAttribute("aria-label")),
      ).toEqual(["Follow requests need approval"]);
    });
  });

  test("a badge the account shows from now on, and not the one it stopped showing", async () => {
    const { user } = await read();

    await user.click(toggle("Show “Supporter”"));
    await waitFor(() => {
      expect(within(profile()).queryAllByText("Supporter")).toHaveLength(1);
    });

    await user.click(toggle("Show “Moderator”"));
    await waitFor(() => {
      expect(within(profile()).queryAllByText("Moderator")).toHaveLength(0);
    });
    expect(within(profile()).queryAllByText("Supporter")).toHaveLength(1);
  });
});

/**
 * A control of each kind of save the screen makes, to say how its row tells of the save: what the reader does, the
 * operation it is sent with, and whether the control shows the choice (`shown`) or the server's value.
 */
const kinds = [
  {
    kind: "a member of the credentials",
    route: "PATCH /accounts/update_credentials",
    act: flip(locked),
    shown: () => toggle(locked).checked,
  },
  {
    kind: "the featured hashtags",
    route: "PUT /featured_tags",
    act: write("sourdough{Enter}"),
    shown: () => featured().includes("sourdough"),
  },
  {
    kind: "a badge",
    route: "PUT /preferences/badges/{role_id}",
    act: flip("Show “Supporter”"),
    shown: () => toggle("Show “Supporter”").checked,
  },
] as const;

describe("the row of a control says how its save goes", () => {
  for (const { kind, route, act: change, shown } of kinds) {
    test(`${kind}: Saving… until the server answers, then Saved, and no other row speaks`, async () => {
      const { fake, user } = await read();
      const sent = fake.script(route, { hold: "request" });
      expect(shown()).toBe(false);

      await change(user);
      await sent.arrived();

      await waitFor(() => {
        expect(said()).toEqual(["Saving…"]);
      });

      sent.release();
      await waitFor(() => {
        expect(said()).toEqual(["Saved"]);
      });
      expect(shown()).toBe(true);
    });

    // A hashtag the reader added is in the list before the server answers, as the switches and the choice show the
    // reader's choice at once: the field is empty by then, and the word would show nowhere.
    test(`${kind}: the control shows the reader's choice at once, before the server read the request`, async () => {
      const { fake, user } = await read();
      const sent = fake.script(route, { hold: "request" });
      await change(user);
      await sent.arrived();
      await waitFor(() => {
        expect(said()).toEqual(["Saving…"]);
      });

      expect(shown()).toBe(true);
    });

    test(`${kind}: a save the server fails puts the control back and says so, and Try again sends the same change`, async () => {
      const { fake, user } = await read();
      const sent = fake.script(route);
      const failing = fake.script(route, { times: 1, fail: 500 });

      await change(user);

      await waitFor(() => {
        expect(said().join()).toContain("This could not be saved.");
      });
      expect(shown()).toBe(false);
      expect(screen.getAllByRole("button", { name: "Try again" })).toHaveLength(1);
      failing.end();

      await user.click(screen.getByRole("button", { name: "Try again" }));

      await waitFor(() => {
        expect(said()).toEqual(["Saved"]);
      });
      expect(shown()).toBe(true);
      expect(sent.seen).toHaveLength(2);
      expect(sent.seen[1]?.body).toEqual(sent.seen[0]?.body);
      expect(sent.seen[1]?.url.pathname).toBe(sent.seen[0]?.url.pathname);
      expect(screen.queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
    });
  }
});

describe("appearing in search", () => {
  // What the server states of search engines, whether it has a search of posts, and the line under the switch.
  const servers = [
    ["allow", true, "Search engines and this server's search of posts may show them."],
    ["allow", false, "Search engines may show them."],
    [
      "deny",
      true,
      "This server tells search engines not to list any account. Your choice still decides whether this server's search of posts shows your posts.",
    ],
    [
      "deny",
      false,
      "This server tells search engines not to list any account. Your choice is still sent to other servers, which may use it for their search.",
    ],
  ] as const;

  for (const [search, posts, line] of servers)
    test(`on a server that ${search === "allow" ? "lets" : "tells"} search engines ${search === "allow" ? "list" : "not to list"} accounts, ${posts ? "with" : "without"} a search of posts, the switch is the reader's and says what it decides`, async () => {
      const { fake, user } = await read({ crawlers: { search }, search: { posts } });
      const sent = fake.script("PATCH /accounts/update_credentials");

      expect(about(toggle(indexable))).toEqual([line]);
      expect(toggle(indexable).disabled).toBe(false);
      // The account's own value, whatever the server tells search engines.
      expect(toggle(indexable).checked).toBe(true);

      await user.click(toggle(indexable));

      expect((await sent.served()).body).toEqual({ indexable: false });
      await waitFor(() => {
        expect(said()).toEqual(["Saved"]);
      });
      expect(toggle(indexable).checked).toBe(false);
    });

  test("an account that stays out of search reads so on a server that tells search engines not to list any account, too", async () => {
    await read({ crawlers: { search: "deny" } }, { indexable: false });

    expect(toggle(indexable).checked).toBe(false);
    expect(toggle(indexable).disabled).toBe(false);
  });
});

describe("refusing AI", () => {
  test("on a server that leaves it to each account, the switch is the reader's and says what it does", async () => {
    await read();

    expect(toggle(ai).disabled).toBe(false);
    expect(toggle(ai).checked).toBe(false);
    expect(about(toggle(ai))).toEqual([
      "When on, your pages on this server tell AI crawlers not to train on them or use them. Other servers and your media files do not carry this.",
    ]);
  });

  test("on a server that refuses AI training for every account, the switch is still the reader's, and says what is left to choose", async () => {
    const { fake, user } = await read({ crawlers: { ai: "deny_training" } });
    const sent = fake.script("PATCH /accounts/update_credentials");

    expect(toggle(ai).disabled).toBe(false);
    expect(toggle(ai).checked).toBe(false);
    expect(about(toggle(ai))).toEqual([
      "This server already refuses AI training for every account. When on, your pages on this server also refuse use by AI search and assistants. Other servers and your media files do not carry this.",
    ]);

    await user.click(toggle(ai));

    expect((await sent.served()).body).toEqual({ ai_opt_out: true });
  });

  for (const own of [false, true])
    test(`on a server that refuses AI for every account, the switch stands on, cannot be changed and says the choice has no effect, whether the account ${own ? "refuses" : "allows"} it`, async () => {
      const { fake, user } = await read({ crawlers: { ai: "deny" } }, { ai_opt_out: own });
      const sent = fake.script("PATCH /accounts/update_credentials");

      expect(toggle(ai).checked).toBe(true);
      expect(toggle(ai).disabled).toBe(true);
      expect(about(toggle(ai))).toEqual([
        "This server already refuses AI training and use for every account, so your choice has no effect.",
      ]);

      await user.click(toggle(ai));
      // The next change the reader makes is the first the server hears of.
      await user.click(toggle(locked));

      expect((await sent.served()).body).toEqual({ locked: true });
      expect(sent.seen).toHaveLength(1);
      expect(toggle(ai).checked).toBe(true);
    });
});

describe("the Atom feed", () => {
  const offers = "People can read your public posts in a feed reader, without an account.";
  const follows = "Until you choose here, this follows the search setting above.";

  test("of an account that never chose follows the search setting, and says so: staying out of search turns it off, with the one request", async () => {
    const { fake, user } = await read();
    const sent = fake.script("PATCH /accounts/update_credentials");
    expect(about(toggle(feed))).toEqual([`${offers} ${follows}`]);
    expect(toggle(feed).checked).toBe(true);

    await user.click(toggle(indexable));

    expect((await sent.served()).body).toEqual({ indexable: false });
    await waitFor(() => {
      expect(toggle(feed).checked).toBe(false);
    });
    // It still follows: the reader chose nothing here.
    expect(about(toggle(feed))).toEqual([`${offers} ${follows}`]);
    expect(sent.seen).toHaveLength(1);

    await user.click(toggle(indexable));

    await waitFor(() => {
      expect(toggle(feed).checked).toBe(true);
    });
    expect(sent.seen.map((request) => request.body)).toEqual([{ indexable: false }, { indexable: true }]);
  });

  test("once the reader chose here, the line no longer says that it follows, and the search setting no longer moves it", async () => {
    const { fake, user } = await read();
    const sent = fake.script("PATCH /accounts/update_credentials");

    await user.click(toggle(feed));
    await sent.served();
    await waitFor(() => {
      expect(about(toggle(feed))).toEqual([offers]);
    });
    expect(toggle(feed).checked).toBe(false);

    await user.click(toggle(feed));
    await sent.served(2);
    await waitFor(() => {
      expect(toggle(feed).checked).toBe(true);
    });
    await user.click(toggle(indexable));
    await sent.served(3);
    await waitFor(() => {
      expect(toggle(indexable).checked).toBe(false);
    });

    expect(toggle(feed).checked).toBe(true);
    expect(about(toggle(feed))).toEqual([offers]);
    expect(sent.seen.map((request) => request.body)).toEqual([
      { feed_enabled: false },
      { feed_enabled: true },
      { indexable: false },
    ]);
  });
});

describe("before the badges and the featured hashtags are read", () => {
  for (const route of ["GET /preferences/badges", "GET /featured_tags"] as const) {
    test(`the screen says that it loads the settings, with no control, until ${route} answers`, async () => {
      const on = fakeData();
      const reading = on.fake.script(route, { hold: "request" });
      await drawn(on);
      await reading.arrived();

      expect(said()).toEqual(["Loading your settings…"]);
      expect(screen.queryAllByRole("switch")).toHaveLength(0);
      expect(screen.queryAllByRole("textbox")).toHaveLength(0);
      expect(screen.queryAllByRole("combobox")).toHaveLength(0);

      reading.release();
      await screen.findByRole("textbox", { name: hashtags });
      // The notice is gone, and no row has anything to say yet.
      expect(said()).toEqual([]);
      expect(toggle("Show “Moderator”").checked).toBe(true);
    });

    test(`when ${route} fails the screen says that the settings could not be loaded, and Try again reads them`, async () => {
      const on = fakeData();
      const reading = on.fake.script(route, { times: 1, fail: 500 });
      const user = await drawn(on);

      const failed = await screen.findByRole("alert");
      expect(failed.textContent).toContain("Your settings could not be loaded.");
      expect(screen.queryAllByRole("switch")).toHaveLength(0);
      expect(screen.queryAllByRole("textbox")).toHaveLength(0);

      await user.click(within(failed).getByRole("button", { name: "Try again" }));

      await screen.findByRole("textbox", { name: hashtags });
      expect(reading.seen).toHaveLength(2);
      expect(screen.queryAllByRole("alert")).toHaveLength(0);
      expect(featured()).toEqual(["baking", "photography"]);
    });
  }
});
