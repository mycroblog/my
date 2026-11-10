// SPDX-License-Identifier: AGPL-3.0-only
// Who may reply to, boost, and quote a post (ADR-0155): the rules of the client API's interaction policy as the form,
// the settings, and the menu of the viewer's own post choose and show them. The server decides every interaction;
// this only names the rules and tells which ones the visibility of a post overrides.
import type { components } from "@mycroblog/mykit";
import type { ClientMessageKey, Translate } from "../../../lib/i18n";

export type InteractionPolicy = components["schemas"]["InteractionPolicy"];
export type InteractionPolicyChange = components["schemas"]["InteractionPolicyChange"];
export type InteractionRule = components["schemas"]["InteractionRule"];
export type InteractionAudience = components["schemas"]["InteractionAudience"];
export type InteractionKind = components["schemas"]["InteractionKind"];
type Visibility = components["schemas"]["Visibility"];

/** The kinds of interaction a policy has, in the order the form and the summary name them. */
export const interactionKinds: readonly InteractionKind[] = ["reply", "announce", "quote"];

/** The audiences, widest first. */
export const interactionAudiences: readonly InteractionAudience[] = ["everyone", "followers", "mentioned", "nobody"];

/** Whether `value` is an audience, for the value of a select. */
export function isInteractionAudience(value: unknown): value is InteractionAudience {
  return interactionAudiences.some((audience) => audience === value);
}

/** The policy that a change makes of `base`: the kinds the change has replace those of `base`. */
export function withChange(base: InteractionPolicy, change: InteractionPolicyChange): InteractionPolicy {
  return {
    reply: change.reply ?? base.reply,
    announce: change.announce ?? base.announce,
    quote: change.quote ?? base.quote,
  };
}

/**
 * The rule with the automatic audience `automatic`, keeping whom the author approves by hand while that is still wider
 * (the server refuses a manual audience that is not wider than the automatic one); none for everyone.
 */
export function withAutomatic(rule: InteractionRule, automatic: InteractionAudience): InteractionRule {
  const manual = rule.manual;
  if (manual === undefined || interactionAudiences.indexOf(manual) >= interactionAudiences.indexOf(automatic)) {
    return { automatic };
  }
  return { automatic, manual };
}

/**
 * The rule that lets the others ask for the author's approval (everyone wider than the automatic audience), or stops
 * it. An automatic audience of everyone leaves no one to ask.
 */
export function withApproval(rule: InteractionRule, asks: boolean): InteractionRule {
  if (!asks || rule.automatic === "everyone") return { automatic: rule.automatic };
  return { automatic: rule.automatic, manual: rule.manual ?? "everyone" };
}

/** Whether a rule admits everyone at once: the default, which the form does not call out. */
export function isOpenRule(rule: InteractionRule): boolean {
  return rule.automatic === "everyone";
}

/**
 * What the visibility of a post decides of `kind` whatever the rules (ADR-0155 §3): a followers-only post is boosted
 * and quoted only by its author (`author`), a direct one by no one (`none`); null when the rules decide.
 */
export function visibilityLimit(kind: InteractionKind, visibility: Visibility): "author" | "none" | null {
  if (kind === "reply") return null;
  if (visibility === "direct") return "none";
  return visibility === "private" ? "author" : null;
}

/** The name of each audience, as a choice. */
export const audienceLabel: Readonly<Record<InteractionAudience, ClientMessageKey>> = {
  everyone: "client:compose.interaction.audience.everyone",
  followers: "client:compose.interaction.audience.followers",
  mentioned: "client:compose.interaction.audience.mentioned",
  nobody: "client:compose.interaction.audience.nobody",
};

/** The label of the choice of each kind's audience. */
export const kindLabel: Readonly<Record<InteractionKind, ClientMessageKey>> = {
  reply: "client:compose.interaction.reply.label",
  announce: "client:compose.interaction.announce.label",
  quote: "client:compose.interaction.quote.label",
};

/** What a rule admits, in words: its automatic audience, and whom the author approves by hand. */
function ruleText(t: Translate, rule: InteractionRule, limit: "author" | "none" | null = null): string {
  if (limit === "author") return t(audienceLabel.nobody);
  if (limit === "none") return t("client:compose.interaction.audience.none");
  const automatic = t(audienceLabel[rule.automatic]);
  return rule.manual === undefined
    ? automatic
    : t("client:compose.interaction.rule.manual", { automatic, manual: rule.manual });
}

/** Who may reply to, boost, and quote a post of `visibility` under `policy`, in one sentence. */
export function policySummary(t: Translate, policy: InteractionPolicy, visibility?: Visibility): string {
  const text = (kind: InteractionKind) =>
    ruleText(t, policy[kind], visibility === undefined ? null : visibilityLimit(kind, visibility));
  return t("client:compose.interaction.summary", {
    reply: text("reply"),
    announce: text("announce"),
    quote: text("quote"),
  });
}
