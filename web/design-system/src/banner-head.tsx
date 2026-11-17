// SPDX-License-Identifier: AGPL-3.0-only

// The head of a page about one thing with a picture: the banner, 3:1, on the raised surface when there is none and
// from edge to edge on a narrow column; the lead (a portrait) over the banner's lower edge, half on it, ringed by the
// page's surface, with what the reader may do at the other end of its row, as far from the banner's edge as from the
// lead; then the rest, a head's gap apart. It decides by the width of its own container, not the viewport's.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";

const head = tv({
  slots: {
    root: "@container grid grid-cols-[minmax(0,1fr)] gap-screen-profile-gap",
    banner: [
      "-mx-shell-edge -mt-shell-gap aspect-banner overflow-hidden bg-surface-raised",
      "@reply:mx-0 @reply:mt-0 @reply:rounded-overlay",
    ],
    image: "block size-full object-cover",
    // The row's start undoes the head's gap, so that the lead's half reaches onto the banner.
    row: "-mt-[calc(var(--avatar-preview)/2+var(--screen-profile-gap))] flex items-start gap-screen-head-room",
    lead: "ms-screen-avatar-inset inline-flex rounded-full shadow-[0_0_0_var(--screen-avatar-ring)_var(--color-surface)]",
    end: "ms-auto mt-[calc(var(--avatar-preview)/2+var(--screen-head-room))] flex items-center gap-screen-tab-gap",
  },
});

/**
 * The banner, then the `lead` over its lower edge at the size of an avatar's preview and `end` at the other end of
 * that row, then `children`, each a head's gap under the one before. `banner` is the picture's address; the raised
 * surface stands in for none. The head is named by `labelledBy`, the id of its heading, and reads in `dir`.
 */
export function BannerHead({
  banner,
  lead,
  end,
  labelledBy,
  dir,
  children,
}: {
  banner?: string | undefined;
  lead: ReactNode;
  end?: ReactNode;
  labelledBy: string;
  dir: "ltr" | "rtl";
  children?: ReactNode;
}) {
  const s = head();
  return (
    <section className={s.root()} aria-labelledby={labelledBy} dir={dir}>
      <div className={s.banner()}>{banner !== undefined && <img className={s.image()} src={banner} alt="" />}</div>
      <div className={s.row()}>
        <span className={s.lead()}>{lead}</span>
        {end !== undefined && <div className={s.end()}>{end}</div>}
      </div>
      {children}
    </section>
  );
}
