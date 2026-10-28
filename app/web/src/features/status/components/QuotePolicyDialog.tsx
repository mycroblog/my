// SPDX-License-Identifier: AGPL-3.0-only
import { Dialog, Radio, RadioGroup } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { quotePolicies, quotePolicyDescription, quotePolicyLabel } from "../../compose";
import type { QuotePolicy } from "../api/status";

export type QuotePolicyDialogProps = {
  value: QuotePolicy;
  onChange: (policy: QuotePolicy) => void;
  isPending: boolean;
  isOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
};

/**
 * Who may quote the viewer's post (FR-114, ADR-0069 §1), changed at once on a choice as the preferences are
 * (the result is a toast). Approvals given before stay; each is revoked from the quoting post.
 */
export function QuotePolicyDialog({ value, onChange, isPending, isOpen, onOpenChange }: QuotePolicyDialogProps) {
  const t = useT();
  return (
    <Dialog
      title={t("client:status.quote_policy.title")}
      closeLabel={t("client:status.quote_policy.close")}
      size="sm"
      isOpen={isOpen}
      onOpenChange={onOpenChange}
    >
      <RadioGroup
        label={t("client:status.quote_policy.label")}
        description={t("client:status.quote_policy.description")}
        value={value}
        isDisabled={isPending}
        onChange={(next) => {
          const policy = quotePolicies.find((p) => p === next);
          if (policy !== undefined) onChange(policy);
        }}
      >
        {quotePolicies.map((policy) => (
          <Radio key={policy} value={policy} description={t(quotePolicyDescription[policy])}>
            {t(quotePolicyLabel[policy])}
          </Radio>
        ))}
      </RadioGroup>
    </Dialog>
  );
}
