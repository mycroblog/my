// SPDX-License-Identifier: AGPL-3.0-only

// The moderators' screens, at /moderation and under it (ADR-0058 §2): the root, which lists those the reader's
// permissions open (moderation.index.tsx), the screen of each entry (moderation.$entry.tsx), and the screens under an
// entry (a report, moderation.reports.$id.tsx). Its loader waits for who the reader is and what waits for them, so
// that the root comes with every screen it lists.

import { loadStaffFacts } from "@my/app-features";
import { createFileRoute, Outlet } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";

export const Route = createFileRoute("/moderation")({
  loader: ({ context }) => screenLoad(context, loadStaffFacts),
  component: Outlet,
  // The screens under it say their own.
  staticData: { scroll: "top" },
});
