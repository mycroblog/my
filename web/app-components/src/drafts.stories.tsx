// SPDX-License-Identifier: AGPL-3.0-only

// The drafts' list (drafts.tsx), its rows holding long text: the sections one under another, and each row's menu at its
// end; the layout tests open the same story (web/e2e/catalog). The words are samples in English.

import { MenuButton, MenuItem } from "@my/design-system";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { DraftCount, DraftList, DraftRow, DraftSection, DraftSections } from "./drafts";

/** A name with no space to break at, longer than a phone's line. */
const longWord = "Donaudampfschifffahrtselektrizitätenhauptbetriebswerkbauunterbeamtengesellschaft";
const sentence =
  "A wide valley at first light, three ridges one behind the other, the nearest dark with pines and the farthest a pale blue line.";
const nothing = () => undefined;

/** The side margin of the app's column around a story, which a row's surface reaches into. */
const column: Decorator = (Story) => (
  <div style={{ padding: "1rem" }}>
    <Story />
  </div>
);

const meta = { title: "Drafts", parameters: { layout: "fullscreen" }, decorators: [column] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The sections of the drafts' list, their rows holding long text. */
export const Drafts: Story = {
  render: () => (
    <DraftSections>
      <DraftSection
        title={
          <>
            Scheduled <DraftCount count={2} />
          </>
        }
        note="Times are in Asia/Tokyo (GMT+9)"
      >
        <DraftList label="Scheduled">
          {["first", "second"].map((which) => (
            <DraftRow
              key={which}
              lead={{ kind: "scheduled", text: `The ${which} goes out on Monday, November 16 at 9:00 AM` }}
              text={`The ${which} scheduled post. ${sentence} ${longWord}`}
              blank="No text"
              warning={which === "first" ? `Spoilers: ${longWord}` : undefined}
              marks={[
                { icon: "attach", label: "2 images", count: 2 },
                { icon: "poll", label: "A poll" },
              ]}
              saved="Saved 2h ago"
              openLabel={`Open the ${which} scheduled post`}
              onOpen={nothing}
              menu={
                <MenuButton icon="more" label={`More for the ${which} scheduled post`}>
                  <MenuItem id="delete" icon="delete">
                    Delete…
                  </MenuItem>
                </MenuButton>
              }
            />
          ))}
        </DraftList>
      </DraftSection>
      <DraftSection
        title={
          <>
            Drafts <DraftCount count={1} />
          </>
        }
      >
        <DraftList label="Drafts">
          <DraftRow
            text=""
            blank="No text"
            marks={[]}
            saved="Saved 3d ago"
            openLabel="Open the draft"
            onOpen={nothing}
            menu={
              <MenuButton icon="more" label="More for the draft">
                <MenuItem id="delete" icon="delete">
                  Delete…
                </MenuItem>
              </MenuButton>
            }
          />
        </DraftList>
      </DraftSection>
    </DraftSections>
  ),
};
