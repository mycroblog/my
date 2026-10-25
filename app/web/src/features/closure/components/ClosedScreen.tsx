// SPDX-License-Identifier: AGPL-3.0-only
// What a signed-in user sees once the server has closed (FR-809, ADR-0088 §2), in place of the app: the closure's
// dates and the operators' message, the archives of the user's data to ask for and download, the CSV files, and
// signing out. The client API refuses everything else, so the screen offers nothing else.
import { Button, PageHeading, Spinner, Stack, StandalonePage } from "@my/ui";
import { Suspense, useEffect } from "react";
import { useIntl } from "react-intl";
import { type Closure } from "../../../lib/closure";
import { useT } from "../../../lib/i18n";
import { useInstanceName } from "../../../lib/instanceName";
import { useSignOut } from "../../session";
import { CsvExportsSection, DataExportsSection } from "../../settings";
import { OperatorMessage } from "./ClosureBanner";

/** The format of the dates of the screen. */
const dateFormat = { dateStyle: "long" } as const;

export function ClosedScreen({ closure }: { closure: Closure }) {
  const t = useT();
  const intl = useIntl();
  const instance = useInstanceName();
  const { signOut, isPending } = useSignOut();
  const title = t("client:closure.closed.title");
  // The screen takes the place of the layout's main, which names the document after the page.
  useEffect(() => {
    document.title = t("client:shell.document_title", { page: title, instance });
  }, [t, title, instance]);
  return (
    <StandalonePage>
      <Stack gap="6">
        <PageHeading>{title}</PageHeading>
        <p>
          {t("client:closure.closed.body", {
            instance,
            date: intl.formatDate(closure.closes_at, dateFormat),
            until: intl.formatDate(closure.export_until, dateFormat),
          })}
        </p>
        <OperatorMessage message={closure.message} />
        {/* The list of archives loads while the screen shows; it waits on its own, so that the rest is there at once. */}
        <Suspense fallback={<Spinner label={t("client:closure.closed.loading")} />}>
          <DataExportsSection />
        </Suspense>
        <CsvExportsSection />
        <div>
          <Button onPress={signOut} isPending={isPending}>
            {t("client:closure.closed.sign_out")}
          </Button>
        </div>
      </Stack>
    </StandalonePage>
  );
}
