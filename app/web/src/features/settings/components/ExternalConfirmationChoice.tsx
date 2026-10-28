// SPDX-License-Identifier: AGPL-3.0-only
// The confirmation of an operation with a linked login provider (FR-010): once the user came back confirmed, a note
// that says with which provider; before that, a button per linked provider that leaves the app to sign in there
// again (provider names are shown as the administrators wrote them, dir="auto"). A failed confirmation is an alert
// (web-client/README.md E2).
import { Button, Cluster, Hint, InlineError, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import type { ExternalConfirmation } from "../hooks/useExternalConfirmation";

export function ExternalConfirmationChoice({ confirmation }: { confirmation: ExternalConfirmation }) {
  const t = useT();
  if (confirmation.confirmed !== undefined) {
    return (
      <Hint>
        {t("client:settings.external_confirmation.confirmed", { provider: confirmation.confirmed.providerName })}
      </Hint>
    );
  }
  return (
    <Stack align="start" gap="3">
      <p>{t("client:settings.external_confirmation.description")}</p>
      <Cluster gap="2">
        {confirmation.providers.map((link) => (
          <Button
            key={link.id}
            variant="secondary"
            isPending={confirmation.startingId === link.provider_id}
            onPress={() => {
              confirmation.start(link.provider_id);
            }}
          >
            <span dir="auto">{t("client:settings.external_confirmation.start", { provider: link.provider_name })}</span>
          </Button>
        ))}
      </Cluster>
      {confirmation.error !== undefined && (
        <InlineError alert size="inherit">
          {confirmation.error}
        </InlineError>
      )}
    </Stack>
  );
}
