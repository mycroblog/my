// SPDX-License-Identifier: AGPL-3.0-only

// The head of a page about one thing with a picture: the banner, 3:1, on the raised surface when there is none and
// from edge to edge on a narrow column; the lead (a portrait) over the banner's lower edge, half on it, ringed by the
// surface it stands on, with what the reader may do at the other end of its row, as far from the banner's edge as
// from the lead; then the rest, a head's gap apart. It decides by the width of its own container, not the viewport's.
// `strip`, the head shown small inside another screen (a profile in its owner's settings): the banner is a strip that
// keeps within the head at every width, and the head is a group, not a region of the page.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";

const head = tv({
  slots: {
    root: "@container grid grid-cols-[minmax(0,1fr)] gap-screen-profile-gap",
    // Without a picture, the banner is a tone of the raised surface: a step off a raised section it stands on.
    banner: "overflow-hidden bg-surface-raised [[data-surface-raised]_&]:bg-surface-raised-hover",
    image: "block size-full object-cover",
    // The row's start undoes the head's gap, so that the lead's half reaches onto the banner.
    row: "-mt-[calc(var(--avatar-preview)/2+var(--screen-profile-gap))] flex items-start gap-screen-head-room",
    lead: [
      "ms-screen-avatar-inset inline-flex rounded-full",
      "shadow-[0_0_0_var(--screen-avatar-ring)_var(--color-surface)]",
      "[[data-surface-raised]_&]:shadow-[0_0_0_var(--screen-avatar-ring)_var(--color-surface-raised)]",
    ],
    end: "ms-auto mt-[calc(var(--avatar-preview)/2+var(--screen-head-room))] flex items-center gap-screen-tab-gap",
  },
  variants: {
    strip: {
      false: { banner: "-mx-shell-edge -mt-shell-gap aspect-banner @reply:mx-0 @reply:mt-0 @reply:rounded-overlay" },
      true: { banner: "aspect-[6/1] rounded-overlay" },
    },
  },
});

/**
 * The banner, then the `lead` over its lower edge at the size of an avatar's preview and `end` at the other end of
 * that row, then `children`, each a head's gap under the one before. `banner` is the picture's address; the raised
 * surface stands in for none. The head is named by `labelledBy`, the id of its name, and reads in `dir`. `strip`: the
 * head inside another screen, its banner a strip within it.
 */
export function BannerHead({
  banner,
  lead,
  end,
  labelledBy,
  dir,
  strip = false,
  children,
}: {
  banner?: string | undefined;
  lead: ReactNode;
  end?: ReactNode;
  labelledBy: string;
  dir: "ltr" | "rtl";
  strip?: boolean | undefined;
  children?: ReactNode;
}) {
  const s = head({ strip });
  const parts = (
    <>
      <div className={s.banner()}>{banner !== undefined && <img className={s.image()} src={banner} alt="" />}</div>
      <div className={s.row()}>
        <span className={s.lead()}>{lead}</span>
        {end !== undefined && <div className={s.end()}>{end}</div>}
      </div>
      {children}
    </>
  );
  return strip ? (
    <div role="group" className={s.root()} aria-labelledby={labelledBy} dir={dir}>
      {parts}
    </div>
  ) : (
    <section className={s.root()} aria-labelledby={labelledBy} dir={dir}>
      {parts}
    </section>
  );
}
