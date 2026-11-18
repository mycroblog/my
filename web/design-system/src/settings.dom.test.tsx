// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a settings screen (#3198, ADR-0110 §5): what the menu, a screen, its sections, a setting's row, a list
// and the submit bar are to assistive technology and to the keyboard, and that the stories made of them keep the
// rules of ADR-0110 §6 (settingsFindings, which has a test of its own). The save state's words and its Try again are
// save-state.dom.test.tsx, the choice and the number field.dom.test.tsx, the switch switch.dom.test.tsx. How the parts
// look, where the menu stands at a width and where the submit bar stays are layout tests
// (web/e2e/catalog/specs/layout/settings.spec.ts).

import { expect, test } from "bun:test";
import { Button, NumberField, SaveState, Select } from "@my/design-system";
import { settingsFindings } from "@my/design-system/test/settings";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import * as partStories from "./settings-parts.stories";
import {
  CopyBlock,
  SettingRow,
  SettingsEmpty,
  SettingsFacts,
  SettingsGroup,
  SettingsItem,
  SettingsList,
  SettingsListRow,
  SettingsMenu,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  SubmitBar,
} from "./settings";
import * as screenStories from "./settings.stories";

/** Whether `first` stands before `second` in the page, as it is read and as Tab goes. */
const before = (first: Element, second: Element) =>
  (first.compareDocumentPosition(second) & first.DOCUMENT_POSITION_FOLLOWING) !== 0;

// ---- The menu -------------------------------------------------------------------------------------------------------

/** A menu of two groups, the reader at Display and posting, and Sign out… at its end. */
function menu({ compact = false }: { compact?: boolean } = {}) {
  render(
    <SettingsMenu
      label="Settings"
      compact={compact}
      end={<SettingsItem icon="signOut" label="Sign out…" count={undefined} href="/settings?sign_out=true" away />}
    >
      <SettingsGroup heading="General">
        <SettingsItem
          icon="profile"
          label="Profile"
          hint="Name, bio, images"
          count={undefined}
          href="/settings/profile"
        />
        <SettingsItem
          icon="display"
          label="Display and posting"
          hint="Language, time zone"
          count={undefined}
          href="/settings/display"
          current
        />
      </SettingsGroup>
      <SettingsGroup heading="Account">
        <SettingsItem
          icon="accountData"
          label="Account and data"
          hint="Export, delete"
          count={3}
          countLabel="3 to read"
          href="/settings/account"
        />
        <SettingsItem icon="invite" label="Invites" count={12} href="/settings/invites" />
      </SettingsGroup>
    </SettingsMenu>,
  );
  return within(screen.getByRole("navigation", { name: "Settings" }));
}

/** Where the links lead, in the order of the page. */
const leads = (links: HTMLElement[]) => links.map((link) => link.getAttribute("href"));

test("the menu is a navigation named by its label, each group a region named by its heading, and its end in no group", () => {
  const nav = menu();

  const general = within(nav.getByRole("region", { name: "General" }));
  expect(general.getByRole("heading", { name: "General" })).not.toBeNull();
  expect(leads(general.getAllByRole("link"))).toEqual(["/settings/profile", "/settings/display"]);
  const account = within(nav.getByRole("region", { name: "Account" }));
  expect(account.getByRole("heading", { name: "Account" })).not.toBeNull();
  expect(leads(account.getAllByRole("link"))).toEqual(["/settings/account", "/settings/invites"]);
  // The groups, then what is apart from them.
  expect(leads(nav.getAllByRole("link"))).toEqual([
    "/settings/profile",
    "/settings/display",
    "/settings/account",
    "/settings/invites",
    "/settings?sign_out=true",
  ]);
  const signOut = nav.getByRole("link", { name: "Sign out…" });
  expect(nav.getAllByRole("region").filter((group) => group.contains(signOut))).toHaveLength(0);
});

test("the entry the reader is at is the menu's current page, and no other entry is", () => {
  const nav = menu();

  expect(leads(nav.getAllByRole("link", { current: "page" }))).toEqual(["/settings/display"]);
});

test("an entry's link says its name, what it holds and its count, a space apart: the count by its label, or the number itself", () => {
  const nav = menu();

  expect(nav.getByRole("link", { name: "Profile Name, bio, images" })).not.toBeNull();
  expect(nav.getByRole("link", { name: "Account and data Export, delete 3 to read" })).not.toBeNull();
  expect(nav.getByRole("link", { name: "Invites 12" })).not.toBeNull();
});

test("a compact menu keeps its groups and its counts, and leaves out what an entry holds", () => {
  const nav = menu({ compact: true });

  expect(nav.getAllByRole("region").map((group) => within(group).getByRole("heading").textContent)).toEqual([
    "General",
    "Account",
  ]);
  expect(nav.getByRole("link", { name: "Profile" })).not.toBeNull();
  expect(nav.getByRole("link", { name: "Display and posting", current: "page" })).not.toBeNull();
  expect(nav.getByRole("link", { name: "Account and data 3 to read" })).not.toBeNull();
  expect(nav.getByRole("link", { name: "Invites 12" })).not.toBeNull();
});

// ---- A screen and its sections ------------------------------------------------------------------------------------

test("a screen shows its title as a heading, and Tab goes from its way back through its sections to its foot", async () => {
  const user = userEvent.setup();
  render(
    <SettingsScreen
      icon="display"
      title="Display and posting"
      back={<Button>Settings</Button>}
      foot={
        <SubmitBar>
          <Button>Save</Button>
        </SubmitBar>
      }
    >
      <SettingsSection heading="Display">
        <Button>First</Button>
      </SettingsSection>
      <SettingsSection heading="New posts">
        <Button>Second</Button>
      </SettingsSection>
    </SettingsScreen>,
  );

  expect(screen.getByRole("heading", { name: "Display and posting" })).not.toBeNull();
  const stops: (string | null | undefined)[] = [];
  for (let i = 0; i < 4; i += 1) {
    await user.tab();
    stops.push(document.activeElement?.textContent);
  }
  expect(stops).toEqual(["Settings", "First", "Second", "Save"]);
});

for (const [where, sub, level] of [
  ["an entry's screen", false, 2],
  ["a screen under an entry", true, 3],
] as const)
  test(`in ${where}, a section and a status summary are regions named by their headings, of level ${String(level)}`, () => {
    render(
      <SettingsScreen title="Security" sub={sub}>
        <StatusSummary
          icon="caution"
          title="Two-factor authentication is off"
          about="A code from your phone stops them."
          action={<Button kind="primary">Turn on</Button>}
        />
        <SettingsSection heading="Sign-in">
          <Button>Password</Button>
        </SettingsSection>
      </SettingsScreen>,
    );

    const summary = within(screen.getByRole("region", { name: "Two-factor authentication is off" }));
    expect(summary.getByRole("heading", { level, name: "Two-factor authentication is off" })).not.toBeNull();
    expect(summary.getByText("A code from your phone stops them.")).not.toBeNull();
    expect(summary.getByRole("button", { name: "Turn on" })).not.toBeNull();
    const section = within(screen.getByRole("region", { name: "Sign-in" }));
    expect(section.getByRole("heading", { level, name: "Sign-in" })).not.toBeNull();
    expect(section.getByRole("button", { name: "Password" })).not.toBeNull();
  });

test("a status summary with nothing to do holds no control", () => {
  render(
    <StatusSummary
      icon="accepted"
      title="Two-factor authentication is on"
      about="Signing in asks for a code from your phone."
    />,
  );

  const summary = within(screen.getByRole("region", { name: "Two-factor authentication is on" }));
  expect(summary.getByText("Signing in asks for a code from your phone.")).not.toBeNull();
  expect(summary.queryAllByRole("button")).toHaveLength(0);
});

test("a section's heading says its count after its name, and the section holds the line about it and its action before what it lists", () => {
  render(
    <SettingsSection
      heading="Passkeys"
      count="2"
      about="Sign in with your device's lock."
      action={<Button>Add a passkey</Button>}
    >
      <SettingsList label="Passkeys">
        <SettingsListRow title="MacBook Touch ID" />
        <SettingsListRow title="Pixel 9" />
      </SettingsList>
    </SettingsSection>,
  );

  const section = within(screen.getByRole("region", { name: /^Passkeys\s*2$/ }));
  expect(section.getByRole("heading", { name: /^Passkeys\s*2$/ })).not.toBeNull();
  expect(section.getByText("Sign in with your device's lock.")).not.toBeNull();
  expect(before(section.getByRole("button", { name: "Add a passkey" }), section.getByRole("list"))).toBe(true);
});

// ---- A list and its empty state -----------------------------------------------------------------------------------

test("a list is named by its label; what stands at a row's end is a group named by the row's title, so one Remove is told from the next", () => {
  render(
    <SettingsList label="Passkeys">
      <SettingsListRow icon="passkey" title="MacBook Touch ID" caption="Added Mar 3" end={<Button>Remove</Button>} />
      <SettingsListRow icon="passkey" title="Pixel 9" caption="Added Jun 12" end={<Button>Remove</Button>} />
    </SettingsList>,
  );

  const list = within(screen.getByRole("list", { name: "Passkeys" }));
  const rows = list.getAllByRole("listitem").map((row) => row.textContent);
  expect(rows).toHaveLength(2);
  expect(rows[0]).toMatch(/^MacBook Touch ID\s*Added Mar 3\s*Remove$/);
  expect(rows[1]).toMatch(/^Pixel 9\s*Added Jun 12\s*Remove$/);
  const removes = ["MacBook Touch ID", "Pixel 9"].map((title) =>
    within(list.getByRole("group", { name: title })).getByRole("button", { name: "Remove" }),
  );
  expect(removes[0] === removes[1]).toBe(false);
  expect(list.getAllByRole("button", { name: "Remove" })).toHaveLength(2);
});

test("a row of a list that has an address is a link, which says its title, its caption and its state", () => {
  render(
    <SettingsList label="Sign-in">
      <SettingsListRow
        icon="time"
        title="Two-factor authentication"
        caption="A code from your phone"
        state="On"
        href="/settings/security/two-factor"
      />
    </SettingsList>,
  );

  const row = within(screen.getByRole("list", { name: "Sign-in" })).getByRole("listitem");
  const link = within(row).getByRole("link", { name: /^Two-factor authentication\s*A code from your phone\s*On$/ });
  expect(link.getAttribute("href")).toBe("/settings/security/two-factor");
});

test("an empty list's place says so, then leads to adding", () => {
  render(
    <SettingsSection heading="Passkeys">
      <SettingsEmpty action={<Button>Add a passkey</Button>}>No passkeys yet.</SettingsEmpty>
    </SettingsSection>,
  );

  const section = within(screen.getByRole("region", { name: "Passkeys" }));
  expect(before(section.getByText("No passkeys yet."), section.getByRole("button", { name: "Add a passkey" }))).toBe(
    true,
  );
  expect(section.queryAllByRole("list")).toHaveLength(0);
});

// ---- Facts, and what a reader copies ------------------------------------------------------------------------------

test("the facts a section states are a list named by its label: a fact an item, the lines about it after it in their order, and nothing to press", () => {
  render(
    <SettingsSection heading="Permissions that need your attention" count="2">
      <SettingsFacts
        label="Permissions that need your attention"
        items={[
          { text: "Read your account, posts, and timelines", about: ["Can read your direct messages"] },
          {
            text: "Post and change things for you",
            about: ["Can post, edit and delete posts as you", "Can change your profile"],
          },
          { text: "Receive push notifications" },
        ]}
      />
    </SettingsSection>,
  );

  // The list is named by its label alone: the count beside the heading is the section's.
  const list = within(screen.getByRole("list", { name: "Permissions that need your attention" }));
  const [read, write, push, ...others] = list.getAllByRole("listitem").map((item) => item.textContent);
  expect(others).toEqual([]);
  expect(read).toMatch(/^Read your account, posts, and timelines\s*Can read your direct messages$/);
  expect(write).toMatch(
    /^Post and change things for you\s*Can post, edit and delete posts as you\s*Can change your profile$/,
  );
  expect(push).toBe("Receive push notifications");
  expect([...list.queryAllByRole("button"), ...list.queryAllByRole("link")]).toHaveLength(0);
});

test("what a reader copies is a list named by its label, a value an item, whole; its action stands after the values, and its result in a status that is there from the start", () => {
  const secret = "my_cs_Jq3vX8nB2mK7pL0wR5tY9uE4iO6aS1dF3gH8jZ2xC7v";
  const block = (result?: string) => (
    <CopyBlock
      label="Client secret"
      values={[secret, "q6b4k-rs"]}
      action={<Button>Copy secret</Button>}
      result={result}
    />
  );
  const { rerender } = render(block());

  const list = screen.getByRole("list", { name: "Client secret" });
  expect(
    within(list)
      .getAllByRole("listitem")
      .map((item) => item.textContent),
  ).toEqual([secret, "q6b4k-rs"]);
  const copy = screen.getByRole("button", { name: "Copy secret" });
  expect(before(list, copy)).toBe(true);
  const place = screen.getByRole("status");
  expect(place.textContent).toBe("");

  rerender(block("Copied"));

  // The same region, so that what comes into it is read.
  expect(screen.getByRole("status") === place).toBe(true);
  expect(place.textContent).toBe("Copied");
  expect(screen.getByRole("button", { name: "Copy secret" }) === copy).toBe(true);
});

// A row of something that does not work now (`inactive`) differs to the eye alone: what a reader of the page's tree
// gets is the row it would be otherwise, and its facts say that it does not work.
test("a row of something that does not work now is the same link by the same name, neither disabled nor hidden: its facts say so", () => {
  render(
    <SettingsList label="Invites">
      <SettingsListRow title="k17wdrqp" facts={["Usable until Nov 25", "Used 0 of 1 time"]} href="/invites/1" />
      <SettingsListRow title="k3np5b83" inactive facts={["Revoked", "Used 0 of 3 times"]} href="/invites/2" />
      <SettingsListRow title="k4vlj2ys" inactive facts={["Expired Nov 15"]} />
    </SettingsList>,
  );

  const list = within(screen.getByRole("list", { name: "Invites" }));
  expect(list.getAllByRole("listitem").map((row) => row.textContent.replace(/\s+/g, " "))).toEqual([
    "k17wdrqp Usable until Nov 25 Used 0 of 1 time",
    "k3np5b83 Revoked Used 0 of 3 times",
    "k4vlj2ys Expired Nov 15",
  ]);
  const works = list.getByRole("link", { name: /^k17wdrqp\s*Usable until Nov 25\s*Used 0 of 1 time$/ });
  const revoked = list.getByRole("link", { name: /^k3np5b83\s*Revoked\s*Used 0 of 3 times$/ });
  expect(revoked.getAttribute("href")).toBe("/invites/2");
  // Nothing but its words tells the one that does not work from the one that does.
  const told = (link: HTMLElement) =>
    link
      .getAttributeNames()
      .filter((name) => name.startsWith("aria-") || name === "tabindex")
      .map((name) => [name, link.getAttribute(name)]);
  expect(told(revoked)).toEqual(told(works));
  expect(list.getAllByRole("link")).toHaveLength(2);
});

// ---- A setting's row ----------------------------------------------------------------------------------------------

const sizes = [
  { value: "large", label: "Large" },
  { value: "small", label: "Small" },
];

// A number's unit stands beside its field and is part of what describes it, after the row's own line: "… days".
for (const [kind, role, field, unit] of [
  ["a choice", "combobox", <Select key="choice" options={sizes} />, ""],
  ["a number", "textbox", <NumberField key="number" min={1} unit="days" />, " days"],
] as const) {
  const then = unit === "" ? "" : ", then its unit";

  test(`${kind} in a setting's row is named by the row's label, described by the line about it${then}, and a press on the name focuses it`, async () => {
    const user = userEvent.setup();
    render(
      <SettingRow id="setting" label="Link previews" about="How large a link's card is in a post" save={null}>
        {field}
      </SettingRow>,
    );
    const control = screen.getByRole(role, {
      name: "Link previews",
      description: `How large a link's card is in a post${unit}`,
    });

    await user.click(screen.getByText("Link previews"));

    expect(document.activeElement === control).toBe(true);
  });

  test(`${kind} in a row that cannot be changed now is disabled and described by the reason, in place of the line about it${then}`, async () => {
    const user = userEvent.setup();
    render(
      <SettingRow
        id="setting"
        label="Link previews"
        about="How large a link's card is in a post"
        isDisabled
        reason="This server shows every card small."
        save={null}
      >
        {field}
      </SettingRow>,
    );
    const control = screen.getByRole<HTMLSelectElement | HTMLInputElement>(role, {
      name: "Link previews",
      description: `This server shows every card small.${unit}`,
    });
    expect(control.disabled).toBe(true);
    expect(screen.queryAllByText("How large a link's card is in a post")).toHaveLength(0);

    await user.click(screen.getByText("Link previews"));
    await user.tab();

    expect(document.activeElement === control).toBe(false);
  });
}

test("a row's save state stands in a status region that is there from the start, and describes the control only while a state stands", () => {
  const row = (save: ReactNode) => (
    <SettingRow id="cards" label="Link previews" about="How large a link's card is in a post" save={save}>
      <Select options={sizes} />
    </SettingRow>
  );
  const described = (description: string | RegExp) =>
    screen.queryAllByRole("combobox", { name: "Link previews", description });
  const { rerender } = render(row(null));
  const place = screen.getByRole("status");
  expect(place.textContent).toBe("");
  expect(described("How large a link's card is in a post")).toHaveLength(1);

  rerender(row(<SaveState state="saved" />));

  // The same region, so that what comes into it is read.
  expect(screen.getByRole("status") === place).toBe(true);
  expect(place.textContent).toBe("Saved");
  expect(described(/^How large a link's card is in a post\s*Saved$/)).toHaveLength(1);

  rerender(row(null));

  expect(screen.getByRole("status") === place).toBe(true);
  expect(place.textContent).toBe("");
  expect(described("How large a link's card is in a post")).toHaveLength(1);
});

test("a number in a row is described by the line about it, then its unit, then its save state while one stands", () => {
  const row = (save: ReactNode) => (
    <SettingRow id="keep" label="Keep posts for" about="Older posts are deleted" save={save}>
      <NumberField min={1} unit="days" />
    </SettingRow>
  );
  const described = (description: string | RegExp) =>
    screen.queryAllByRole("textbox", { name: "Keep posts for", description });
  const { rerender } = render(row(<SaveState state="saved" />));

  expect(described(/^Older posts are deleted\s+days\s+Saved$/)).toHaveLength(1);

  rerender(row(null));

  // The state gone, the unit stays: it is the field's, not the save's.
  expect(described(/^Older posts are deleted\s+days$/)).toHaveLength(1);
});

test("a row with no line about it describes its control by its save state alone, and by nothing at rest", () => {
  const row = (save: ReactNode) => (
    <SettingRow id="zone" label="Time zone" save={save}>
      <Select options={sizes} />
    </SettingRow>
  );
  const { rerender } = render(row(null));
  expect(screen.getByRole("combobox", { name: "Time zone" }).getAttribute("aria-describedby")).toBeNull();

  rerender(row(<SaveState state="failed" onRetry={() => undefined} />));

  expect(
    screen.getByRole("combobox", { name: "Time zone", description: /^This could not be saved\.\s*Try again$/ }),
  ).not.toBeNull();
});

test("a row of a form saved as a whole has no place for a save state", () => {
  render(
    <form aria-label="Filter">
      <SettingRow id="action" label="Posts that match" about="What a match does" form>
        <Select name="action" options={sizes} />
      </SettingRow>
    </form>,
  );

  expect(screen.getByRole("combobox", { name: "Posts that match", description: "What a match does" })).not.toBeNull();
  expect(screen.queryAllByRole("status")).toHaveLength(0);
  expect([...new FormData(screen.getByRole<HTMLFormElement>("form", { name: "Filter" })).entries()]).toEqual([
    ["action", "large"],
  ]);
});

// A value the server refused when the form was sent is said at its row (`error`), where the reader changes it.
for (const [kind, role, field] of [
  ["a choice", "combobox", <Select key="choice" name="value" options={sizes} />],
  ["a number", "textbox", <NumberField key="number" name="value" min={1} max={10} defaultValue={3} />],
] as const) {
  /** The row in a form saved as a whole, with the line about it or without, and what is wrong with its value. */
  const row = (given: { about?: string; error?: string }) => (
    <form aria-label="Invite">
      <SettingRow id="refused" label="Works for" form {...given}>
        {field}
      </SettingRow>
    </form>
  );
  /** What describes the control, part by part, in the order it is read. */
  const described = (control: HTMLElement) =>
    (control.getAttribute("aria-describedby") ?? "")
      .split(" ")
      .filter((id) => id !== "")
      .map((id) => document.getElementById(id)?.textContent ?? "");
  const about = "Up to 30 days.";
  const error = "This server no longer allows that long. Choose a shorter time.";

  test(`${kind} in a row of a form whose value was refused is marked invalid and described by the line about it, then by what is wrong, which stands under that line and is no alert`, () => {
    const { rerender } = render(row({ about }));
    const control = screen.getByRole(role, { name: "Works for" });
    expect(control.getAttribute("aria-invalid")).not.toBe("true");
    expect(described(control)).toEqual([about]);

    rerender(row({ about, error }));

    expect(screen.getByRole(role, { name: "Works for" }) === control).toBe(true);
    expect(control.getAttribute("aria-invalid")).toBe("true");
    expect(described(control)).toEqual([about, error]);
    expect(before(screen.getByText(about), screen.getByText(error))).toBe(true);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
    expect(screen.queryAllByRole("status")).toHaveLength(0);

    // Sent again: the row is as it was before the refusal.
    rerender(row({ about }));

    expect(control.getAttribute("aria-invalid")).not.toBe("true");
    expect(described(control)).toEqual([about]);
    expect(screen.queryAllByText(error)).toHaveLength(0);
  });

  test(`${kind} in a row with no line about it is described by what is wrong alone`, () => {
    render(row({ error }));

    const control = screen.getByRole(role, { name: "Works for" });
    expect(control.getAttribute("aria-invalid")).toBe("true");
    expect(described(control)).toEqual([error]);
  });
}

// ADR-0110 §6: a control that saves at once takes its save state as a required prop. The compiler holds it
// (web:typecheck reads this file): each line below is an error, and stops being one if the prop becomes optional.
test("a setting's row that saves at once cannot be written without its save state", () => {
  const without = [
    // @ts-expect-error -- no `save`, and not a row of a form.
    <SettingRow key="row" id="zone" label="Time zone">
      <Select options={sizes} />
    </SettingRow>,
    // @ts-expect-error -- `form` says the row is saved with its form: it takes no save state.
    <SettingRow key="both" id="zone" label="Time zone" form save={null}>
      <Select options={sizes} />
    </SettingRow>,
  ];

  expect(without).toHaveLength(2);
});

// ---- The submit bar -----------------------------------------------------------------------------------------------

test("the submit bar's result stands in a status region that is there without a result, before the button", () => {
  const bar = (result?: ReactNode) => (
    <SubmitBar result={result}>
      <Button kind="primary">Save</Button>
    </SubmitBar>
  );
  const { rerender } = render(bar());
  const place = screen.getByRole("status");
  expect(place.textContent).toBe("");
  expect(before(place, screen.getByRole("button", { name: "Save" }))).toBe(true);

  rerender(bar(<SaveState state="saved" />));

  expect(screen.getByRole("status") === place).toBe(true);
  expect(place.textContent).toBe("Saved");
});

// ---- The stories ----------------------------------------------------------------------------------------------------

/** A story as this test draws it: what its `render` returns, which takes nothing. */
type Drawn = { render?: (...args: never[]) => ReactNode };

/** The stories of a file by their export's name, without its `default`. */
const storiesOf = (file: Record<string, unknown>) =>
  Object.entries(file).flatMap(([name, story]) => (name === "default" ? [] : [[name, story as Drawn] as const]));

// ChoiceAndNumber shows rows on the page, beside a section, to show the fields on both surfaces: no screen is so.
const loose = new Set(["ChoiceAndNumber"]);

for (const [file, stories] of [
  ["Settings", storiesOf(screenStories)],
  ["Settings parts", storiesOf(partStories).filter(([name]) => !loose.has(name))],
] as const)
  for (const [name, story] of stories)
    test(`the story ${file} / ${name} keeps the rules of a settings screen (ADR-0110 §6)`, () => {
      cleanup();
      if (story.render === undefined) throw new Error(`test: the story ${name} has no render`);
      const { container } = render(<>{story.render()}</>);

      // It drew something to check.
      expect(within(container).queryAllByRole("heading").length).toBeGreaterThan(0);
      expect(settingsFindings(container)).toEqual([]);
    });
