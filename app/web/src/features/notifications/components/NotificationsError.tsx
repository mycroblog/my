// SPDX-License-Identifier: AGPL-3.0-only
import { type HeadingLevel, RegionError } from "@my/ui";
import { useState } from "react";
import { useT } from "../../../lib/i18n";

type NotificationsErrorProps = {
  /** One below the heading of the region it fills. */
  headingLevel: HeadingLevel;
  message: string;
  retry: () => void;
  isRetrying: boolean;
};

/**
 * The failure of the notifications in place of the list (web-client/16-look-states-and-wording.md G16). The failure the page opened with is read, not
 * announced; a retry that fails again is the result of the user's action, and interrupts as an alert.
 */
export function NotificationsError({ headingLevel, message, retry, isRetrying }: NotificationsErrorProps) {
  const t = useT();
  const [retried, setRetried] = useState(false);
  return (
    <RegionError
      headingLevel={headingLevel}
      heading={t("client:notifications.failed")}
      reason={message}
      retryLabel={t("client:notifications.retry")}
      onRetry={() => {
        setRetried(true);
        retry();
      }}
      isRetrying={isRetrying}
      alert={retried && !isRetrying}
    />
  );
}
