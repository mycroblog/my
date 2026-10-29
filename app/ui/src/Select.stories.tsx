// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Button } from "./Button";
import { ImageIcon, LanguageIcon } from "./Icons";
import { Select, SelectItem } from "./Select";

const languages = [
  { id: "en", name: "English" },
  { id: "ja", name: "日本語" },
  { id: "ar", name: "العربية" },
  { id: "de", name: "Deutsch" },
];

const meta = {
  title: "Forms/Select",
  component: Select<(typeof languages)[number]>,
  args: {
    label: "Post language",
    placeholder: "Choose a language",
    items: languages,
    children: (language) => <SelectItem id={language.id}>{language.name}</SelectItem>,
  },
} satisfies Meta<typeof Select<(typeof languages)[number]>>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Placeholder: Story = {};

export const Selected: Story = { args: { defaultValue: "ja", description: "Readers can filter posts by language." } };

export const Invalid: Story = { args: { isInvalid: true, errorMessage: "Choose a language." } };

export const Disabled: Story = { args: { defaultValue: "en", isDisabled: true } };

/**
 * A button in a row of tools, as tall as the ghost `toolbar` Button beside it: no visible label (the
 * `aria-label` names it), the icon and the chosen option.
 */
export const Toolbar: Story = {
  args: {
    label: undefined,
    "aria-label": "Post language",
    variant: "toolbar",
    icon: <LanguageIcon />,
    defaultValue: "ja",
  },
  render: (args) => (
    <div className="flex flex-wrap items-center gap-1">
      <Button variant="ghost" size="toolbar" aria-label="Add images">
        <ImageIcon />
      </Button>
      <Select {...args} />
    </div>
  ),
};

/**
 * The toolbar select with a tooltip like the icon buttons beside it: it names the control and its current value
 * on hover and keyboard focus, closes on Esc and stays closed while the list is open. The accessible name does
 * not change.
 */
export const ToolbarTooltip: Story = {
  args: {
    label: undefined,
    "aria-label": "Post language",
    variant: "toolbar",
    icon: <LanguageIcon />,
    defaultValue: "ja",
    tooltip: "Post language: Japanese",
  },
  render: (args) => (
    <div className="flex flex-wrap items-center gap-1">
      <Button variant="ghost" size="toolbar" aria-label="Add images">
        <ImageIcon />
      </Button>
      <Select {...args} />
    </div>
  ),
  play: async ({ canvas, userEvent }) => {
    // The first stop is the button before the select.
    await userEvent.tab();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: /Post language/ })).toHaveFocus();
    await expect(await canvas.findByRole("tooltip")).toHaveTextContent("Post language: Japanese");
  },
};

/** The open list, for the accessibility check of the options; it renders in a popover outside the frame. */
export const Open: Story = { args: { defaultValue: "ja", defaultOpen: true } };

const zones = [
  { id: "America/New_York", name: "(UTC−04:00) Eastern Time" },
  { id: "America/Detroit", name: "(UTC−04:00) Eastern Time" },
  { id: "Asia/Tokyo", name: "(UTC+09:00) Japan Standard Time" },
];

/**
 * Options told apart by a description, open; the closed select shows only the label. An option's name is all of
 * its visible text, the label then the description (WCAG 2.5.3, label in name).
 */
export const Descriptions: Story = {
  args: {
    label: "Time zone",
    defaultValue: "America/Detroit",
    defaultOpen: true,
    items: zones,
    children: (zone) => (
      <SelectItem id={zone.id} textValue={zone.name} description={zone.id}>
        {zone.name}
      </SelectItem>
    ),
  },
  play: async ({ canvas, canvasElement }) => {
    const listbox = await within(canvasElement.ownerDocument.body).findByRole("listbox");
    await expect(
      within(listbox).getByRole("option", { name: "(UTC−04:00) Eastern Time America/Detroit" }),
    ).toBeInTheDocument();
    // The closed select names and shows the label alone.
    await expect(canvas.getByRole("button", { name: /Time zone/ })).toHaveAccessibleName(
      "(UTC−04:00) Eastern Time Time zone",
    );
  },
};
