// SPDX-License-Identifier: AGPL-3.0-only

// Proves settingsFindings, the check of ADR-0110 §6 on a rendered tree, as restrictions.test.js proves the lint: it
// finds each break of a rule in a tree that has it, and nothing in a tree that keeps them.

import { expect, test } from "bun:test";
import { render } from "@testing-library/react";
import { Button } from "../button";
import { CheckField } from "../form-page";
import {
  SettingRow,
  SettingsEmpty,
  SettingsGroup,
  SettingsItem,
  SettingsList,
  SettingsListRow,
  SettingsMenu,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  SubmitBar,
} from "../settings";
import { Select } from "../field";
import { Switch } from "../switch";
import { SETTINGS_MOST, settingsFindings } from "./settings";

const options = [{ value: "a", label: "A" }];
const items = (count: number) =>
  Array.from({ length: count }, (_, i) => (
    <SettingsItem key={i} icon="profile" label={`Entry ${String(i + 1)}`} count={undefined} href={`/${String(i)}`} />
  ));
const switches = (count: number) =>
  Array.from({ length: count }, (_, i) => <Switch key={i} label={`Switch ${String(i + 1)}`} save={null} />);

test("a menu and screens that keep the rules have no finding", () => {
  const { container } = render(
    <>
      <SettingsMenu label="Settings" end={items(1)}>
        <SettingsGroup heading="General">{items(SETTINGS_MOST)}</SettingsGroup>
      </SettingsMenu>
      <SettingsMenu label="Settings" compact>
        <SettingsGroup heading="General">{items(SETTINGS_MOST)}</SettingsGroup>
      </SettingsMenu>
      <SettingsScreen title="Display" back={<Button>Settings</Button>}>
        <StatusSummary icon="caution" title="Something is to do" about="Why" action={<Button>Do it</Button>} />
        <SettingsSection heading="Six and a list" action={<Button>Add</Button>}>
          {switches(SETTINGS_MOST - 2)}
          <SettingsList label="A list is one">
            {Array.from({ length: 9 }, (_, i) => (
              <SettingsListRow key={i} title={`Row ${String(i)}`} end={<Button>Remove</Button>} />
            ))}
          </SettingsList>
        </SettingsSection>
        <SettingsSection heading="Empty">
          <SettingsEmpty action={<Button>Add</Button>}>Nothing yet.</SettingsEmpty>
        </SettingsSection>
      </SettingsScreen>
      <form>
        <SettingsScreen
          sub
          title="Edit"
          foot={
            <SubmitBar>
              <Button type="submit">Save</Button>
            </SubmitBar>
          }
        >
          <SettingsSection heading="Where">
            <CheckField id="home" name="context" value="home" label="Home" />
            <SettingRow id="action" label="Action" form>
              <Select name="action" options={options} />
            </SettingRow>
          </SettingsSection>
        </SettingsScreen>
      </form>
    </>,
  );

  expect(settingsFindings(container)).toEqual([]);
});

test("a group of the menu over the most entries is found, by its name", () => {
  const { container } = render(
    <SettingsMenu label="Settings">
      <SettingsGroup heading="General">{items(SETTINGS_MOST + 1)}</SettingsGroup>
      <SettingsGroup heading="Account">{items(2)}</SettingsGroup>
    </SettingsMenu>,
  );

  expect(settingsFindings(container)).toEqual(['the group "General" of the menu has 8 entries, over 7']);
});

test("a section that holds more than the most is found, its own action among what it holds", () => {
  const { container } = render(
    <SettingsScreen title="Display">
      <SettingsSection heading="Eight">{switches(SETTINGS_MOST + 1)}</SettingsSection>
      <SettingsSection heading="Seven and an action" action={<Button>Add</Button>}>
        {switches(SETTINGS_MOST)}
      </SettingsSection>
      <SettingsSection heading="Seven">{switches(SETTINGS_MOST)}</SettingsSection>
    </SettingsScreen>,
  );

  expect(settingsFindings(container)).toEqual([
    'the section "Eight" holds 8 things to do, over 7',
    'the section "Seven and an action" holds 8 things to do, over 7',
  ]);
});

test("a control beside the sections, in none of them, is found; words beside them are not", () => {
  const { container } = render(
    <SettingsScreen title="Display">
      <SettingsSection heading="Display">{switches(1)}</SettingsSection>
      <p>Words between the sections.</p>
      <div>
        <Button>Reset everything</Button>
      </div>
    </SettingsScreen>,
  );

  expect(settingsFindings(container)).toEqual([
    'a control stands beside the sections, in none of them: "Reset everything"',
  ]);
});

test("a switch in a form that has a submit button is found, in a form and in a screen that ends with a submit bar", () => {
  const { container } = render(
    <>
      <form>
        <Switch label="In a form" save={null} />
        <Button type="submit">Save</Button>
      </form>
      <SettingsScreen
        sub
        title="Edit"
        foot={
          <SubmitBar>
            <Button>Save</Button>
          </SubmitBar>
        }
      >
        <SettingsSection heading="Where">
          <Switch label="In a screen" save={null} />
        </SettingsSection>
      </SettingsScreen>
      <form>
        <Switch label="With no submit button" save={null} />
        <Button>Close</Button>
      </form>
    </>,
  );

  expect(settingsFindings(container)).toEqual([
    'the switch "In a form" stands in a form that has a submit button: a switch saves as it is pressed',
    'the switch "In a screen" stands in a form that has a submit button: a switch saves as it is pressed',
  ]);
});

test("a section that adds from its head and from its empty list is found", () => {
  const { container } = render(
    <SettingsSection heading="Passkeys" action={<Button>Add a passkey</Button>}>
      <SettingsEmpty action={<Button>Add a passkey</Button>}>No passkeys yet.</SettingsEmpty>
    </SettingsSection>,
  );

  expect(settingsFindings(container)).toEqual([
    'the section "Passkeys" adds from its head and from its empty list: one place adds',
  ]);
});
