// SPDX-License-Identifier: AGPL-3.0-only

// The follow at the end of an account's row, in the reader's language (the toolbar's locale), in each state of the
// reader's relationship with it: Follow, Follow back (it follows the reader), Request to follow (it approves its
// followers), Following, and Requested; each row as every list of accounts shows it (a search's, follows', a
// notification's).

import { RowItem, RowList } from "@my/design-system";
import { images } from "@my/design-system/samples";
import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AccountFollow, AccountLine, type FollowState } from "./account";
import { accountOf } from "./post.samples";

const rem = (px: number) => `${px / rootPx}rem`;

const rows: { account: ReturnType<typeof accountOf> & { locked?: boolean }; relationship: FollowState }[] = [
  {
    account: accountOf({ name: "Alice Chen", handle: "@alice@example.social", avatar: images.dusk }),
    relationship: {},
  },
  {
    account: accountOf({ name: "佐藤 美香", handle: "@mika@mstdn.example.jp", avatar: images.snow }),
    relationship: { followed_by: true },
  },
  { account: { ...accountOf({ name: "Ravi", handle: "@ravi@fedi.example.net" }), locked: true }, relationship: {} },
  { account: accountOf({ name: "Lena Novak", handle: "@lena@toot.example.eu" }), relationship: { following: true } },
  {
    account: accountOf({
      name: "Bartholomew Fitzgerald-Montgomery",
      handle: "@bartholomew@a-rather-long-instance-name.example.social",
    }),
    relationship: { requested: true },
  },
];

function States() {
  return (
    <div style={{ maxWidth: rem(600), padding: rem(16) }}>
      <RowList label="Accounts">
        {rows.map(({ account, relationship }) => (
          <RowItem key={account.id}>
            <AccountLine
              account={account}
              end={<AccountFollow account={account} relationship={relationship} onFollow={() => Promise.resolve()} />}
            />
          </RowItem>
        ))}
      </RowList>
    </div>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Account follow", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const EveryState: Story = { render: () => <States /> };
