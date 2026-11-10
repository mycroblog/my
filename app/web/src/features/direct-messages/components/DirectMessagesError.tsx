// SPDX-License-Identifier: AGPL-3.0-only
import { type HeadingLevel, RegionError } from "@my/ui";
import { useT } from "../../../lib/i18n";

type DirectMessagesErrorProps = {
  /** One below the heading of the region it fills. */
  headingLevel: HeadingLevel;
  message: string;
  retry: () => void;
  isRetrying: boolean;
};

/** The failure of the direct messages in place of the list (web-client/16-look-states-and-wording.md G16). */
export function DirectMessagesError({ headingLevel, message, retry, isRetrying }: DirectMessagesErrorProps) {
  const t = useT();
  return (
    <RegionError
      headingLevel={headingLevel}
      heading={t("client:direct_messages.failed")}
      reason={message}
      retryLabel={t("client:direct_messages.retry")}
      onRetry={retry}
      isRetrying={isRetrying}
    />
  );
}
