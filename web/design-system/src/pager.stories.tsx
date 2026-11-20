// SPDX-License-Identifier: AGPL-3.0-only

// The pager (pager.tsx) in each of its states: Previous and Next under a list that is a screen's subject, on its first
// page, a middle one and its last, the next page on its way and failed, with and without the page's position; the
// same part under a window of months, as the finance screens page, in the reader's locale; and Show more under a
// list inside a detail, at rest, busy, failed, and gone once the list is read to its end, then a list that grows under
// it, the focus going to the first item each press added. The items, the addresses and the months are samples.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Box, Stack } from "./layout";
import { Pager, ShowMore, useAddedFocus } from "./pager";
import { dateFormat } from "./zone";

const rem = (px: number) => `${px / rootPx}rem`;
const none = () => undefined;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

/** The states of a story, well apart, so that each reads as its own. */
function Cases({ children }: { children: ReactNode }) {
  return <div style={{ display: "flex", flexDirection: "column", gap: rem(40) }}>{children}</div>;
}

/** One state of the story, named over it for whoever judges it; not a part of the pager. */
function Case({ name, children }: { name: string; children: ReactNode }) {
  return (
    <Stack gap="screen-row-gap">
      <Box text="caption" tone="muted">
        {name}
      </Box>
      {children}
    </Stack>
  );
}

/** A list inside a detail: a webhook's deliveries, a line each. */
function Deliveries({ count }: { count: number }) {
  return (
    <Stack gap="screen-row-gap">
      {Array.from({ length: count }, (_, i) => (
        <Delivery key={i} n={i} />
      ))}
    </Stack>
  );
}

function Delivery({ n }: { n: number }) {
  return (
    <Stack gap="entry-line">
      <span>account.created · 200 OK</span>
      <Box text="caption" tone="muted">
        {`${String(n * 7 + 3)} min ago · 120 ms`}
      </Box>
    </Stack>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Pager", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * Previous and Next: the first page, Previous disabled where it stands; a middle page; the last, Next disabled, where
 * a press on Next that reached it keeps the focus; Next on its way; Next failed, the same button named Try again
 * under the sentence that says so; and a middle page whose position the app does not say.
 */
export const Pages: Story = {
  render: () => (
    <Column>
      <Cases>
        <Case name="First page">
          <Pager next={{ href: "#/moderation/reports?after=1035" }} position={1} />
        </Case>
        <Case name="A middle page">
          <Pager
            previous={{ href: "#/moderation/reports?before=1034" }}
            next={{ href: "#/moderation/reports?after=990" }}
            position={3}
          />
        </Case>
        <Case name="Last page">
          <Pager previous={{ href: "#/moderation/reports?before=989" }} position={7} />
        </Case>
        <Case name="Loading the next page">
          <Pager previous={{ onPress: none }} next={{ onPress: none }} busy="next" position={3} />
        </Case>
        <Case name="The next page failed">
          <Pager previous={{ onPress: none }} next={{ onPress: none }} failed="next" position={3} />
        </Case>
        <Case name="A middle page, its position not said">
          <Pager previous={{ onPress: none }} next={{ onPress: none }} />
        </Case>
      </Cases>
    </Column>
  ),
};

/** A window of months as the finance screens say it: the first month to the last, in the reader's locale. */
function Months({ from, to }: { from: string; to: string }) {
  const { locale } = useLocale();
  // Months of the calendar, kept in UTC, as the finance screens count them.
  const format = dateFormat(locale, undefined, { year: "numeric", month: "short", timeZone: "UTC" });
  return <>{format.formatRange(new Date(`${from}-01T00:00:00Z`), new Date(`${to}-01T00:00:00Z`))}</>;
}

/**
 * The same part under a window of months (the finance screens): Later toward today, Earlier back, the window's months
 * after them, on a line of their own where the line has no room; the window that ends with this month, Later disabled
 * where it stands; one further back.
 */
export const Periods: Story = {
  render: () => (
    <Column>
      <Cases>
        <Case name="Ending this month">
          <Pager periods="month" next={{ onPress: none }} position={<Months from="2026-06" to="2026-11" />} />
        </Case>
        <Case name="Further back">
          <Pager
            periods="month"
            previous={{ onPress: none }}
            next={{ onPress: none }}
            position={<Months from="2025-12" to="2026-05" />}
          />
        </Case>
      </Cases>
    </Column>
  ),
};

/**
 * Show more under a list inside a detail: at rest; the next items on their way; failed, the same button named Try
 * again under the sentence that says so.
 */
export const More: Story = {
  render: () => (
    <Column>
      <Cases>
        <Case name="More to show">
          <Stack gap="entry-room">
            <Deliveries count={2} />
            <ShowMore more onPress={none} />
          </Stack>
        </Case>
        <Case name="Loading more">
          <Stack gap="entry-room">
            <Deliveries count={2} />
            <ShowMore more busy onPress={none} />
          </Stack>
        </Case>
        <Case name="Loading more failed">
          <Stack gap="entry-room">
            <Deliveries count={2} />
            <ShowMore more failed onPress={none} />
          </Stack>
        </Case>
      </Cases>
    </Column>
  ),
};

/**
 * A list inside a detail read to its end: no Show more and no line that says so; its last item is the end, and the
 * detail's next section follows at the space between sections.
 */
export const NothingMore: Story = {
  render: () => (
    <Column>
      <Cases>
        <Stack gap="entry-room">
          <Box text="body" weight="strong">
            Deliveries
          </Box>
          <Deliveries count={2} />
          <ShowMore more={false} onPress={none} />
        </Stack>
        <Stack gap="entry-room">
          <Box text="body" weight="strong">
            Signing secret
          </Box>
          <Box text="caption" tone="muted">
            Rotated 3 days ago
          </Box>
        </Stack>
      </Cases>
    </Column>
  ),
};

/**
 * A list that grows under Show more, three items a press: the focus goes to the first item each press added, the
 * second press fails once and leaves the focus on Try again, and Show more goes once the list is read to its end.
 */
export const Growing: Story = {
  render: () => {
    function List() {
      const [shown, setShown] = useState(3);
      const [busy, setBusy] = useState(false);
      const [failed, setFailed] = useState(false);
      const [presses, setPresses] = useState(0);
      const added = useAddedFocus(shown);
      const load = () => {
        setBusy(true);
        setTimeout(() => {
          setBusy(false);
          setPresses((p) => p + 1);
          if (presses === 1) {
            setFailed(true);
            return;
          }
          setFailed(false);
          setShown((n) => n + 3);
        }, 600);
      };
      return (
        <Stack gap="entry-room">
          <Stack gap="screen-row-gap">
            {Array.from({ length: shown }, (_, i) => (
              <div key={i} ref={added.item(i)} tabIndex={-1}>
                <Delivery n={i} />
              </div>
            ))}
          </Stack>
          <ShowMore more={shown < 12} busy={busy} failed={failed} onPress={added.press(load)} />
        </Stack>
      );
    }
    return (
      <Column>
        <List />
      </Column>
    );
  },
};
