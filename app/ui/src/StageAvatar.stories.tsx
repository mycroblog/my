// SPDX-License-Identifier: AGPL-3.0-only
// StageAvatar stories: the speakers of a room (the host talking, a reaction, microphones off), the listeners with
// raised hands, the menu of the host's operations from an avatar, and long names that are cut short.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Menu, MenuItem } from "./Menu";
import { StageAvatar, StageGrid } from "./StageAvatar";

const labels = { speaking: "Talking", muted: "Microphone off", handRaised: "Hand raised" };

const meta = {
  title: "Voice/StageAvatar",
  component: StageAvatar,
  args: { name: "Haruka", seed: "1", caption: "Host", labels },
  render: (args) => (
    <StageGrid aria-label="Speakers">
      <StageAvatar {...args} />
    </StageGrid>
  ),
} satisfies Meta<typeof StageAvatar>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The speakers: the host talking (ringed), a co-host who reacted, and two speakers whose microphones are off. */
export const Speakers: Story = {
  render: () => (
    <StageGrid aria-label="Speakers">
      <StageAvatar name="Haruka" seed="1" caption="Host" speaking labels={labels} />
      <StageAvatar name="Sota" seed="2" caption="Co-host" reaction={{ content: "❤️", label: "Reacted with ❤️" }} labels={labels} />
      <StageAvatar name="Mio" seed="3" caption="Speaker" muted labels={labels} />
      <StageAvatar name="Kenji" seed="4" caption="Speaker" muted labels={labels} />
    </StageGrid>
  ),
  play: async ({ canvas }) => {
    const list = canvas.getByRole("list", { name: "Speakers" });
    await expect(list).toBeVisible();
    // The ring and the badges are also words for assistive technology.
    const items = canvas.getAllByRole("listitem");
    await expect(items[0]).toHaveTextContent("HarukaHostTalking");
    await expect(items[1]).toHaveTextContent("Reacted with ❤️");
    await expect(items[2]).toHaveTextContent("Microphone off");
  },
};

/** The listeners, small and without a role; two have raised a hand, and one of them reacted too. */
export const Listeners: Story = {
  render: () => (
    <StageGrid aria-label="Listeners" size="listener">
      <StageAvatar size="listener" name="Yui" seed="5" labels={labels} />
      <StageAvatar size="listener" name="Riku" seed="6" handRaised labels={labels} />
      <StageAvatar size="listener" name="Aoi" seed="7" labels={labels} />
      <StageAvatar size="listener" name="Hinata" seed="8" handRaised reaction={{ content: "👏", label: "Reacted with 👏" }} labels={labels} />
      <StageAvatar size="listener" name="Koharu" seed="9" labels={labels} />
      <StageAvatar size="listener" name="Takumi" seed="10" labels={labels} />
    </StageGrid>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getAllByText("Hand raised")).toHaveLength(2);
  },
};

/** The host's operations open from the avatar: a menu button named by the participant and their states. */
export const WithMenu: Story = {
  decorators: [
    (Story) => (
      <div className="min-h-64">
        <Story />
      </div>
    ),
  ],
  args: {
    name: "Mio",
    caption: "Speaker",
    muted: true,
    menu: (
      <Menu aria-label="Mio">
        <MenuItem id="listener">Move to listeners</MenuItem>
        <MenuItem id="mute">Mute</MenuItem>
        <MenuItem id="remove" intent="danger">
          Remove from the room
        </MenuItem>
      </Menu>
    ),
  },
  play: async ({ canvas, userEvent }) => {
    const button = canvas.getByRole("button", { name: /Mio/ });
    await expect(button).toHaveAccessibleName(expect.stringMatching(/Speaker.*Microphone off/));
    await userEvent.tab();
    await expect(button).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    const menu = await canvas.findByRole("menu");
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Move to listeners" })).toHaveFocus());
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(menu).not.toBeInTheDocument());
    await waitFor(() => expect(button).toHaveFocus());
  },
};

/** A long name and a long role are cut short within the column. */
export const LongName: Story = {
  args: { name: "Haruka with a display name long enough to be cut short", caption: "Co-host of the room for tonight", speaking: true },
};
