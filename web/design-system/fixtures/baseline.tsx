// SPDX-License-Identifier: AGPL-3.0-only
// The baseline of restrict/layout-parts and restrict/viewport-breakpoints (web/devtools/src/styles-baseline.json),
// which names this file: a declaration fails with all it wrote when it has more than its baseline allows, and the
// baseline fails when it allows more than there is. web/devtools/src/restrictions.test.js lints it.

import type { ReactNode } from "react"; // error: layout-baseline gone.base
import { tv } from "tailwind-variants/lite";

// As many as its baseline allows (2).
const kept = tv({ base: "flex gap-feed text-body" });
// One more than its baseline allows (1): both fail.
const grown = tv({
  base: [
    "flex text-body", // error: layout flex
    "gap-feed", // error: layout gap-feed
  ],
});
// One fewer than its baseline allows (2), and `gone`, which the baseline names and nothing writes.
const shrunk = tv({ base: "flex text-body" }); // error: layout-baseline shrunk.base
// Under a breakpoint: as many as its baseline allows (1), and one more than it allows (0).
const wide = tv({
  slots: {
    kept: "tablet:text-title",
    grown: "desktop:text-title", // error: viewport desktop:text-title
  },
});

export function Baseline({ children }: { children: ReactNode }) {
  const s = wide();
  return (
    <div className={kept()}>
      <div className={grown()}>
        <div className={shrunk()}>
          <span className={s.kept()}>{children}</span>
          <span className={s.grown()}>{children}</span>
        </div>
      </div>
    </div>
  );
}
