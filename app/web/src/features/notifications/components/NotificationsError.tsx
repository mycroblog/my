// SPDX-License-Identifier: AGPL-3.0-only
import { type HeadingLevel, RegionError } from "@my/ui";
import { useT } from "../../../lib/i18n";

type NotificationsErrorProps = {
  /** One below the heading of the region it fills. */
  headingLevel: HeadingLevel;
  message: string;
  retry: () => void;
  isRetrying: boolean;
};

/** The failure of the notifications in place of the list (web-client/16-look-states-and-wording.md G16). */
export function NotificationsError({ headingLevel, message, retry, isRetrying }: NotificationsErrorProps) {
  const t = useT();
  return (
    <RegionError
      headingLevel={headingLevel}
      heading={t("client:notifications.failed")}
      reason={message}
      retryLabel={t("client:notifications.retry")}
      onRetry={retry}
      isRetrying={isRetrying}
    />
  );
}
