// SPDX-License-Identifier: AGPL-3.0-only

// What a control that saves at once says of its last change, in the design system's own words and the reader's
// language (the toolbar's locale): nothing, saving, saved, not saved with Try again, and a value the server refused,
// in the server's words. The setting's own words are placeholders.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Select } from "./field";
import { useSaveState, type Saving } from "./saving";
import { SettingRow, SettingsSection } from "./settings";
import { Switch } from "./switch";

const rem = (px: number) => `${px / rootPx}rem`;

const sizes = [
  { value: "large", label: "Large" },
  { value: "small", label: "Small" },
];

const states: readonly (Saving | null)[] = [
  null,
  { state: "saving" },
  { state: "saved" },
  { state: "failed", retry: () => undefined },
  { state: "failed", said: "This value is not valid. Check what you entered." },
];

function Row({ at, saving }: { at: number; saving: Saving | null }) {
  const id = `saving-${String(at)}`;
  return (
    <SettingRow id={id} label="Link previews in posts" save={useSaveState(saving)}>
      <Select labelledBy={id} options={sizes} />
    </SettingRow>
  );
}

function Toggle({ saving }: { saving: Saving | null }) {
  return <Switch label="Show new posts as they arrive" save={useSaveState(saving)} defaultSelected />;
}

function All() {
  return (
    <div style={{ maxWidth: rem(600), padding: rem(16), display: "grid", gap: rem(16) }}>
      <SettingsSection heading="Display">
        {states.map((saving, i) => (
          <Row key={i} at={i} saving={saving} />
        ))}
      </SettingsSection>
      <SettingsSection heading="Timelines">
        {states.map((saving, i) => (
          <Toggle key={i} saving={saving} />
        ))}
      </SettingsSection>
    </div>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Saving", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const States: Story = { render: () => <All /> };
