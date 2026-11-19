// SPDX-License-Identifier: AGPL-3.0-only

// The server's administration, at /admin and under it (ADR-0058 §2): the root, which lists the screens the reader's
// permissions open (admin.index.tsx), and the screen of each entry (admin.$entry.tsx). Its loader waits for who the
// reader is, so that the root comes with every screen it lists.

import { loadStaffFacts } from "@my/app-features";
import { createFileRoute, Outlet } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";

export const Route = createFileRoute("/admin")({
  loader: ({ context }) => screenLoad(context, loadStaffFacts),
  component: Outlet,
  // The screens under it say their own.
  staticData: { scroll: "top" },
});
