// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Hint, Link, RegionError, Section, Spinner, Stack, Text } from "@my/ui";
import { useState } from "react";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { useSignOut } from "../hooks/useSignOut";
import { useViewer } from "../hooks/useViewer";

/** The account section of the settings page: who is signed in and the sign-out button, or a link to sign in. */
export function AccountSection() {
  const t = useT();
  return (
    <Section title={t("client:session.account.title")}>
      <AccountStatus />
    </Section>
  );
}

function AccountStatus() {
  const t = useT();
  const viewer = useViewer();
  const { signOut, isPending } = useSignOut();
  const [retried, setRetried] = useState(false);
  switch (viewer.status) {
    case "loading":
      return <Spinner label={t("client:shell.loading")} />;
    case "signed-out":
      return (
        <Stack align="start" gap="2">
          <Hint size="inherit">{t("client:session.signed_out")}</Hint>
          <Link href="/login">{t("client:session.sign_in")}</Link>
        </Stack>
      );
    case "error":
      return (
        <Stack align="start" gap="2">
          {/* Read with the page, not announced; a retry that fails again is the user's action, and an alert (G16). */}
          <RegionError
            headingLevel={3}
            heading={t("client:session.account.failed")}
            reason={viewer.message}
            retryLabel={t("client:session.account.retry")}
            onRetry={() => {
              setRetried(true);
              viewer.retry();
            }}
            isRetrying={viewer.isRetrying}
            alert={retried && !viewer.isRetrying}
          />
          <Button onPress={signOut} isPending={isPending}>
            {t("client:session.sign_out")}
          </Button>
        </Stack>
      );
    case "signed-in": {
      const handle = handleOf(viewer.account.acct);
      return (
        <Stack align="start" gap="2">
          {/* The display name and handle are the user's text, in any language (web-client/README.md Y10). */}
          <Stack as="p">
            <Text dir="auto" weight="semibold" align="matchParent">
              {viewer.account.display_name || viewer.account.username}
            </Text>
            <Hint as="span" dir="ltr">
              {handle}
            </Hint>
          </Stack>
          <Button onPress={signOut} isPending={isPending}>
            {t("client:session.sign_out")}
          </Button>
        </Stack>
      );
    }
  }
}
