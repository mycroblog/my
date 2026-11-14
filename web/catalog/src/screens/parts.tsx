// SPDX-License-Identifier: AGPL-3.0-only

// The rough parts the three screens of #3047 share, which the design system has not yet: the tabs over a list (the
// notifications' filters, a profile's posts), a group's heading in a list (new and earlier notifications, the replies
// of a post), a sentence with a name drawn apart in it, and an account's row (who followed, who asks to). Their look
// is screens.css's.

import { Avatar, Byline } from "@my/design-system";
import type { ReactNode } from "react";
import type { Who } from "./samples";

/** The tabs over a list: one is current; `end` sits at the row's end (a settings link). */
export function Tabs<T extends string>({
  label,
  tabs,
  current,
  onChange,
  end,
}: {
  label: string;
  tabs: readonly { id: T; name: string }[];
  current: T;
  onChange: (id: T) => void;
  end?: ReactNode;
}) {
  return (
    <div className="sc-tabs">
      <div role="tablist" aria-label={label} className="sc-tablist">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={tab.id === current}
            className="sc-tab"
            onClick={() => {
              onChange(tab.id);
            }}
          >
            {tab.name}
          </button>
        ))}
      </div>
      {end !== undefined && <span className="sc-tabs-end">{end}</span>}
    </div>
  );
}

/** A heading of a group of a list, muted and small, as the column beside the main one heads its sections. */
export function GroupHeading({ children }: { children: string }) {
  return <h2 className="sc-group-heading">{children}</h2>;
}

/** The account's name, as a post shows it. */
export const nameOf = (who: Who) => who.name;

/** `template` with `{name}` drawn as the account's name, strong and isolated. */
export function Sentence({ template, who }: { template: string; who: Who }) {
  const [before = "", after = ""] = template.split("{name}");
  return (
    <>
      {before}
      <bdi className="sc-name">{nameOf(who)}</bdi>
      {after}
    </>
  );
}

/** An account on a row of its own: its avatar, its name over its handle, and what the reader may do at the end. */
export function AccountRow({ who, end }: { who: Who; end?: ReactNode }) {
  return (
    <div className="sc-account">
      <Byline
        avatar={<Avatar name={who.name} src={who.avatar} size="post" decorative />}
        name={who.name}
        handle={who.handle}
      />
      {end !== undefined && <div className="sc-pair">{end}</div>}
    </div>
  );
}
