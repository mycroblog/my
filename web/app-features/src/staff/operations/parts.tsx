// SPDX-License-Identifier: AGPL-3.0-only

// What the operations screens share: a count in the reader's locale; the button after a fact that copies its value (an
// ID the logs are searched by, a key, a fingerprint), with what came of copying; and the head of a screen under an
// entry, its way back the reader's only one, as every screen under an entry of the administration has it.

import { BackButton, BackLead, Box, Row, ScreenTitle, TextButton, useLocale, type IconName } from "@my/design-system";
import { copyText } from "@my/platform";
import { useState, type ReactNode } from "react";
import { useStaffStrings } from "../parts";

/** Writes a count in the reader's locale (1,204). */
export function useCount(): (n: number) => string {
  const { locale } = useLocale();
  const format = new Intl.NumberFormat(locale);
  return (n) => format.format(n);
}

/** The button that copies `value`, named `label` (Copy the ID), and what came of copying, read as it comes. */
export function CopyAction({ label, value }: { label: string; value: string }): ReactNode {
  const { t } = useStaffStrings();
  const [copied, setCopied] = useState<boolean>();
  return (
    <Row as="span" inline wrap align="baseline" gap="entry-caption">
      <TextButton
        onPress={() => {
          void copyText(value).then(setCopied);
        }}
      >
        {label}
      </TextButton>
      <Box as="span" role="status" text="caption" tone="muted">
        {copied === undefined ? "" : t(copied ? "staff_settings.copied" : "staff_settings.copy_failed")}
      </Box>
    </Row>
  );
}

/** The head of a screen under an entry: its glyph and title under the way back, `back`'s words, which `onBack` takes. */
export function SubTitle({
  glyph,
  back,
  onBack,
  children,
}: {
  glyph: IconName;
  back: string;
  onBack: () => void;
  children: string;
}): ReactNode {
  return (
    <ScreenTitle
      glyph={glyph}
      back={
        <BackLead value>
          <BackButton onPress={onBack}>{back}</BackButton>
        </BackLead>
      }
    >
      {children}
    </ScreenTitle>
  );
}
