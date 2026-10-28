// SPDX-License-Identifier: AGPL-3.0-only
// The region of a remote account's or server's page that refetches its stored posts from their servers (FR-407,
// federation.md §6.4), for a viewer with manage_domains: what the refetch does, its button, and what the request
// did (a StatusMessage). The button stays, so focus stays on it.
import { Button, Section, Hint, Stack, StatusMessage } from "@my/ui";
import { useId } from "react";
import { useT } from "../../../lib/i18n";
import { type RefetchTarget, useRefetch } from "../hooks/useRefetch";

export type RefetchStatusesSectionProps = { target: Exclude<RefetchTarget, { kind: "status" }> };

export function RefetchStatusesSection({ target }: RefetchStatusesSectionProps) {
  const t = useT();
  const descriptionId = useId();
  const refetch = useRefetch();
  return (
    <Section surface="card" title={t("client:admin.refetch.title")}>
      <StatusMessage tone="success">{refetch.status}</StatusMessage>
      <Stack align="start" gap="1">
        <Button
          isPending={refetch.isPending(target)}
          onPress={() => {
            refetch.request(target);
          }}
          aria-describedby={descriptionId}
        >
          {t("client:admin.refetch.button")}
        </Button>
        <Hint id={descriptionId}>
          {t(
            target.kind === "account"
              ? "client:admin.refetch.account.description"
              : "client:admin.refetch.domain.description",
          )}
        </Hint>
      </Stack>
    </Section>
  );
}
