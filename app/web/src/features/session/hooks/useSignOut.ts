// SPDX-License-Identifier: AGPL-3.0-only
import { useToast } from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useSession } from "../../../lib/auth/SessionProvider";
import { SignOutError } from "../../../lib/auth/session";
import { useT } from "../../../lib/i18n";

/**
 * Returns `signOut`, which signs out in every tab (the session revokes the tokens; the app then drops the
 * cached queries and the Service Worker's caches) and opens the landing page (`/login` would sign in again at
 * once), and whether it is running.
 */
export function useSignOut(): { signOut: () => void; isPending: boolean } {
  const session = useSession();
  const navigate = useNavigate();
  const toast = useToast();
  const t = useT();
  const [isPending, setIsPending] = useState(false);
  const signOut = () => {
    setIsPending(true);
    void session
      .signOut()
      .catch((error: unknown) => {
        if (!(error instanceof SignOutError)) throw error;
        toast.show({ message: t("client:session.sign_out.failed"), tone: "danger", timeout: null });
      })
      .finally(() => {
        setIsPending(false);
        void navigate({ to: "/" });
      });
  };
  return { signOut, isPending };
}
