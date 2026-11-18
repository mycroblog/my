// SPDX-License-Identifier: AGPL-3.0-only

// /settings/import-export, the address the closure's emails carry (internal/app/identity/closure_mail.go): it leads to
// Export, the screen under the settings' entry Account and data where the reader takes their data away, in place of
// this address in the history.

import { dataExportPath } from "@my/app-components";
import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/settings/import-export")({
  beforeLoad: () => {
    redirect({ to: dataExportPath, replace: true, throw: true });
  },
  // The screen it leads to opens at its top.
  staticData: { scroll: "top" },
});
