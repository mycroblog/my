// SPDX-License-Identifier: AGPL-3.0-only
// The checks of ADR-0110 §6 on what a layer above @my/design-system writes of the settings: no switch in a form that
// has a submit button (restrict/switch-in-form), a settings screen of sections alone and no control beside a section
// (restrict/settings-sections), no size on a heading of the second level (restrict/heading-size), and seven things at
// most in a group of the menu and in a section (restrict/settings-count). What a list is mapped to is counted in the
// rendered tree (settingsFindings of @my/design-system/test/settings), not here.
// web/devtools/src/restrictions.test.js lints it; the lint of the package leaves it out.

import * as ui from "@my/design-system";
import {
  Box,
  Button,
  CheckField,
  Form,
  SaveState,
  Select,
  SettingRow,
  SettingsGroup,
  SettingsItem,
  SettingsList,
  SettingsListRow,
  SettingsMenu,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  SubmitBar,
  Switch,
} from "@my/design-system";
import type { ReactNode } from "react";

const save = <SaveState state="saved" />;
const options = [{ value: "a", label: "A" }];

// A screen whose controls save at once: its switches stand in sections, with no submit button.
export const AtOnce = ({ extra }: { extra: boolean }) => (
  <SettingsScreen title="Display">
    <StatusSummary icon="caution" title="Something is to do" about="What it means" />
    <SettingsSection heading="Timelines">
      <Switch label="Show new posts as they arrive" save={save} />
      <SettingRow id="zone" label="Time zone" save={null}>
        <Select options={options} />
      </SettingRow>
    </SettingsSection>
    {extra && (
      <SettingsSection heading="More">
        <Switch label="Ask for a description of media" save={null} />
      </SettingsSection>
    )}
    <Own />
  </SettingsScreen>
);

// A component of the feature's own passes: what it renders is its own file's to keep.
function Own() {
  return (
    <SettingsSection heading="Own">
      <Switch label="Own" save={null} />
    </SettingsSection>
  );
}

// A form saved as a whole takes checkboxes; a switch saves as it is pressed.
export const Whole = () => (
  <Form action="/settings/filters">
    <CheckField id="whole" name="whole" value="1" label="Whole word" />
    <Switch label="Whole word" save={null} /> {/* error: switch <Switch> */}
    <ui.Switch label="Home and lists" save={null} /> {/* error: switch <Switch> */}
    <input type="checkbox" role="switch" /> {/* error: switch role="switch" */}
    <Button type="submit">Save</Button>
  </Form>
);

export const Native = ({ children }: { children: ReactNode }) => (
  <form>
    <div>
      <span role="switch" aria-checked="false" tabIndex={0} /> {/* error: switch role="switch" */}
    </div>
    {children}
    <button type="submit">Save</button>
  </form>
);

// A form without a submit button posts nothing as a whole: its switches save at once.
export const Open = () => (
  <form>
    <Switch label="Boosts" save={null} />
    <button type="button">Close</button>
  </form>
);

// A settings screen that ends with a submit bar is a form saved as a whole, whatever holds its <form>.
export const Filter = () => (
  <SettingsScreen
    sub
    title="Edit filter"
    foot={
      <SubmitBar>
        <Button kind="primary">Save</Button>
      </SubmitBar>
    }
  >
    <SettingsSection heading="Where it applies">
      <Switch label="Notifications" save={null} /> {/* error: switch <Switch> */}
      <SettingRow id="action" label="Posts that match" form>
        <Select name="action" options={options} />
      </SettingRow>
    </SettingsSection>
  </SettingsScreen>
);

// A settings screen takes sections alone.
export const Loose = ({ note }: { note: boolean }) => (
  <SettingsScreen title="Display">
    <SettingsSection heading="Display">
      <Switch label="Inside" save={null} />
    </SettingsSection>
    <Switch label="Outside" save={null} /> {/* error: section <Switch> */}
    <Box>Words between the sections</Box> {/* error: section <Box> */}
    <p>Words</p> {/* error: section <p> */}
    {note && <Button>Reset</Button>} {/* error: section <Button> */}
    <>
      <SettingsSection heading="More">
        <Switch label="Inside" save={null} />
      </SettingsSection>
      <ui.Select options={options} label="Outside" /> {/* error: section <ui.Select> */}
    </>
  </SettingsScreen>
);

// Under any parent, a control does not stand beside a section; words and what is no control may.
export const Mixed = () => (
  <Box>
    <Box>Words above the sections</Box>
    <SettingsSection heading="Display">
      <Switch label="Inside" save={null} />
    </SettingsSection>
    <Select label="Outside" options={options} /> {/* error: section <Select> */}
    <SettingsList label="Outside" /> {/* error: section <SettingsList> */}
    <button type="button">Outside</button> {/* error: section <button> */}
  </Box>
);

// Controls with no section beside them are another screen's matter.
export const Bare = () => (
  <Box>
    <Switch label="A switch" save={null} />
    <Button>A button</Button>
  </Box>
);

// A heading of the second level takes no size.
declare function Heading(props: { level: 1 | 2 | 3 | "2"; size?: string; children: ReactNode }): ReactNode;

export const Headings = () => (
  <>
    <Heading level={2}>Plain</Heading>
    <Heading level={3} size="title">
      Another level
    </Heading>
    <Heading level={2} size="title" children="Sized" /> {/* error: heading <Heading> */}
    <Heading level="2" size="title" children="Its level a string" /> {/* error: heading <Heading> */}
    <h2 size="title">Sized</h2> {/* error: heading <h2> */}
    <h2>Plain</h2>
  </>
);

// A group of the menu has seven entries at most, and a section seven things to do, its action among them.
const entry = (n: number) => <SettingsItem icon="profile" label={`Entry ${String(n)}`} count={undefined} href="/" />;

export const Menu = ({ more, entries }: { more: boolean; entries: number[] }) => (
  <SettingsMenu label="Settings">
    <SettingsGroup heading="Seven">
      <SettingsItem icon="profile" label="1" count={undefined} href="/1" />
      <SettingsItem icon="profile" label="2" count={undefined} href="/2" />
      <SettingsItem icon="profile" label="3" count={undefined} href="/3" />
      <SettingsItem icon="profile" label="4" count={undefined} href="/4" />
      <SettingsItem icon="profile" label="5" count={undefined} href="/5" />
      <SettingsItem icon="profile" label="6" count={undefined} href="/6" />
      {more ? (
        <SettingsItem icon="profile" label="7" count={undefined} href="/7" />
      ) : (
        <SettingsItem icon="profile" label="Seven" count={undefined} href="/seven" />
      )}
    </SettingsGroup>
    <SettingsGroup heading="Eight">
      <SettingsItem icon="profile" label="1" count={undefined} href="/1" />
      <SettingsItem icon="profile" label="2" count={undefined} href="/2" />
      <>
        <SettingsItem icon="profile" label="3" count={undefined} href="/3" />
        {more && <SettingsItem icon="profile" label="4" count={undefined} href="/4" />}
      </>
      <SettingsItem icon="profile" label="5" count={undefined} href="/5" />
      <SettingsItem icon="profile" label="6" count={undefined} href="/6" />
      {more ? <SettingsItem icon="profile" label="7" count={undefined} href="/7" /> : null}
      <SettingsItem icon="profile" label="8" count={undefined} href="/8" /> {/* error: count <SettingsGroup> */}
      <SettingsItem icon="profile" label="9" count={undefined} href="/9" />
    </SettingsGroup>
    {/* What a list is mapped to is not counted here. */}
    <SettingsGroup heading="Mapped">{entries.map(entry)}</SettingsGroup>
  </SettingsMenu>
);

export const Sections = () => (
  <SettingsScreen title="Display">
    <SettingsSection heading="Seven" action={<Button>Add</Button>}>
      <Switch label="1" save={null} />
      <Switch label="2" save={null} />
      <Switch label="3" save={null} />
      <Switch label="4" save={null} />
      <Switch label="5" save={null} />
      <SettingsList label="Seven">
        <SettingsListRow title="A list is one, however many its rows" />
        <SettingsListRow title="2" />
        <SettingsListRow title="3" />
        <SettingsListRow title="4" />
        <SettingsListRow title="5" />
        <SettingsListRow title="6" />
        <SettingsListRow title="7" />
        <SettingsListRow title="8" />
      </SettingsList>
    </SettingsSection>
    <SettingsSection heading="Eight, its action the eighth" action={<Button>Add</Button>}>
      <Switch label="1" save={null} />
      <Switch label="2" save={null} />
      <Switch label="3" save={null} />
      <Switch label="4" save={null} />
      <Switch label="5" save={null} />
      <Switch label="6" save={null} />
      <Switch label="7" save={null} /> {/* error: count <SettingsSection> */}
    </SettingsSection>
    <SettingsSection heading="Eight">
      <Switch label="1" save={null} />
      <Switch label="2" save={null} />
      <Switch label="3" save={null} />
      <Switch label="4" save={null} />
      <Switch label="5" save={null} />
      <Switch label="6" save={null} />
      <Switch label="7" save={null} />
      <Switch label="8" save={null} /> {/* error: count <SettingsSection> */}
    </SettingsSection>
  </SettingsScreen>
);
