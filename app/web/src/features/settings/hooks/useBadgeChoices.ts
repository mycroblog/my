// SPDX-License-Identifier: AGPL-3.0-only
// The badges on the user's profile (ADR-0071): one switch per role with a badge that the user holds, saved as soon
// as it changes. A badge is hidden until the user shows it.
import { type BadgeColor, useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { badgeChoicesQueryOptions, cacheBadgeChoice, updateBadgeChoiceMutation } from "../api/badges";

/** One badge as its switch shows it. */
export type BadgeSwitch = {
  roleId: string;
  /** The staff's name of the badge, shown as user content. */
  label: string;
  color: BadgeColor;
  /** Whether it is shown: the value being saved while pending (web-client/README.md S8), the saved one otherwise. */
  displayed: boolean;
  isPending: boolean;
  /** Why the last change of this badge was not saved, localized; undefined when it was. */
  error: string | undefined;
  onChange: (displayed: boolean) => void;
};

/** The badges that the user may show, highest role first; the route's loader has loaded them. */
export function useBadgeChoices(): BadgeSwitch[] {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const { data } = useSuspenseQuery(badgeChoicesQueryOptions(api));
  const mutation = useMutation({
    ...updateBadgeChoiceMutation(api),
    onSuccess: async (saved) => {
      await cacheBadgeChoice(api, queryClient, saved);
      const badge = `⁨${saved.label}⁩`;
      announce(
        t(saved.displayed ? "client:settings.profile.badges.shown" : "client:settings.profile.badges.hidden", {
          badge,
        }),
      );
    },
  });
  return data.items.map((choice) => {
    const mine = mutation.variables?.roleId === choice.role_id;
    return {
      roleId: choice.role_id,
      label: choice.label,
      color: choice.color,
      displayed: mine && mutation.isPending ? mutation.variables.displayed : choice.displayed,
      isPending: mine && mutation.isPending,
      error: mine && mutation.isError ? errorMessage(mutation.error) : undefined,
      onChange: (displayed) => {
        mutation.mutate({ roleId: choice.role_id, displayed });
      },
    };
  });
}
