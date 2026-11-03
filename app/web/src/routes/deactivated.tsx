// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, Link } from "@tanstack/react-router";
import { Stack } from "@my/ui";
import { useIntl } from "react-intl";
import { z } from "zod";
import { useT } from "../lib/i18n";
import { InfoLayout } from "./-layout/InfoLayout";
import { PageHeading } from "./-layout/PageHeading";

/** When the account is deleted, as the server answered the deactivation; anything else says no date (web-client/README.md S3). */
const searchSchema = z.object({
  deletion_due_at: z.iso.datetime().optional().catch(undefined),
});

/**
 * What a user reads once they have deactivated their account and are signed out (ADR-0145 §3): the date of the deletion,
 * and how to come back.
 */
export const Route = createFileRoute("/deactivated")({
  staticData: { title: "client:deactivated.title", back: true },
  validateSearch: searchSchema,
  component: DeactivatedPage,
});

function DeactivatedPage() {
  const t = useT();
  const intl = useIntl();
  const { deletion_due_at: due } = Route.useSearch();
  return (
    <InfoLayout heading={<PageHeading>{t("client:deactivated.title")}</PageHeading>}>
      <Stack gap="4">
        {due !== undefined && (
          <p>
            <time dateTime={due}>
              {t("client:deactivated.deletion", {
                date: intl.formatDate(due, { dateStyle: "long", timeStyle: "short" }),
              })}
            </time>
          </p>
        )}
        <p>{t("client:deactivated.description")}</p>
        <p>
          <Link to="/login">{t("client:deactivated.sign_in")}</Link>
        </p>
      </Stack>
    </InfoLayout>
  );
}
