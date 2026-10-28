// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { useInstanceName } from "../../../lib/instanceName";
import { useSignIn } from "../hooks/useSignIn";

export type SignInPanelProps = {
  /** The path of this app to open after signing in. */
  returnTo: string;
};

/** The body of the sign-in page: what signing in gives, and the button to the server's sign-in screen. */
export function SignInPanel({ returnTo }: SignInPanelProps) {
  const t = useT();
  const instanceName = useInstanceName();
  const { signIn, isPending } = useSignIn(returnTo);
  return (
    <Stack align="start" gap="4">
      <p>{t("client:login.body", { instance: instanceName })}</p>
      <Button variant="primary" onPress={signIn} isPending={isPending}>
        {t("client:login.submit")}
      </Button>
    </Stack>
  );
}
