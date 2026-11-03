// SPDX-License-Identifier: AGPL-3.0-only
// The prompt to turn on Web Push, a notice at the top of the notifications page (ADR-0137,
// web-client/screens/notifications.md): a region of the page, never a dialog, so the list stays usable under it.
import { Button, Cluster, InlineError, Section } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { usePushPrompt } from "../hooks/usePushPrompt";

export type PushPromptProps = {
  /** Called just before the prompt goes away: its buttons go too, so the page moves the focus. */
  onGone: () => void;
};

export function PushPrompt({ onGone }: PushPromptProps) {
  const t = useT();
  const prompt = usePushPrompt(onGone);
  if (prompt.status === "hidden") return null;
  const title = t("client:settings.push_prompt.title");
  const closeLabel = t("client:settings.push_prompt.close");
  if (prompt.status === "blocked") {
    return (
      <Section surface="band" title={title}>
        <InlineError alert>{t("client:settings.push.denied")}</InlineError>
        <Cluster>
          {/* The button that was pressed is gone, so the focus comes to the only one left. */}
          <Button size="sm" variant="ghost" autoFocus onPress={prompt.close}>
            {closeLabel}
          </Button>
        </Cluster>
      </Section>
    );
  }
  return (
    <Section surface="band" title={title} description={t("client:settings.push_prompt.description")}>
      {prompt.error !== undefined && <InlineError alert>{prompt.error}</InlineError>}
      <Cluster>
        <Button size="sm" isPending={prompt.isPending} onPress={prompt.enable}>
          {t("client:settings.push_prompt.enable")}
        </Button>
        <Button size="sm" variant="ghost" isDisabled={prompt.isPending} onPress={prompt.close}>
          {closeLabel}
        </Button>
      </Cluster>
    </Section>
  );
}
