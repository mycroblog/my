// SPDX-License-Identifier: AGPL-3.0-only
// A decision's page below its heading (ADR-0147 §9): what the review decided, on what, in which mode, how long it took,
// its reason code and note, then its cancellation. Cancelling takes effect at once and asks nothing first, as in the
// list. What it did stays in the cancellation region in a StatusMessage (role="status"), and focus moves to that
// region's heading, since its button gives way to the sentence that the decision is cancelled. A decision that cannot be cancelled
// says why: it had no effect, or it is a refused local post, which its author appeals.
import { Button, Hint, Section, Stack, StatusMessage } from "@my/ui";
import { useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import type { HookDecision } from "../api/hookDecisions";
import {
  hookNameLabels,
  hookOutcomeLabels,
  type OverrideHookDecision,
  useHookDecision,
  useOverrideHookDecision,
} from "../hooks/useHookDecisions";
import { HookDecisionFacts } from "./hookDecisionParts";
import { RequirePermission, useFocusAfterChange } from "./parts";

export function HookDecisionDetailView({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_reports">
      <HookDecisionPage id={id} />
    </RequirePermission>
  );
}

function HookDecisionPage({ id }: { id: string }) {
  const t = useT();
  const decision = useHookDecision(id);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const expectFocus = useFocusAfterChange(headingRef, decision.overridden_at);
  const [status, setStatus] = useState("");
  const override = useOverrideHookDecision((cancelled) => {
    expectFocus();
    setStatus(
      t("client:admin.hook_decisions.overridden", {
        hook: t(hookNameLabels[cancelled.hook]),
        outcome: t(hookOutcomeLabels[cancelled.outcome]),
      }),
    );
  });
  return (
    <Stack gap="8">
      <Section surface="card" title={t("client:admin.hook_decision.facts.title")}>
        <HookDecisionFacts decision={decision} />
      </Section>
      <Section surface="card" title={t("client:admin.hook_decision.cancel.title")} headingRef={headingRef}>
        <StatusMessage>{status}</StatusMessage>
        <Cancellation decision={decision} override={override} />
      </Section>
    </Stack>
  );
}

/** The cancellation of a decision: its date, the button, or why there is none. */
function Cancellation({ decision, override }: { decision: HookDecision; override: OverrideHookDecision }) {
  const t = useT();
  // When it was cancelled is among the facts above.
  if (decision.overridden_at !== undefined) return <Hint>{t("client:admin.hook_decision.cancelled")}</Hint>;
  if (!decision.overridable) {
    const appealed = decision.hook === "local.status" && decision.outcome === "reject";
    return (
      <Hint>{t(appealed ? "client:admin.hook_decision.appeal_only" : "client:admin.hook_decision.no_effect")}</Hint>
    );
  }
  return (
    <Stack align="start" gap="3">
      <Hint>{t("client:admin.hook_decision.cancel.description")}</Hint>
      <Button
        isPending={override.pendingIds.includes(decision.id)}
        onPress={() => {
          override.override(decision);
        }}
      >
        {t("client:admin.hook_decisions.override")}
      </Button>
    </Stack>
  );
}
