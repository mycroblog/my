// SPDX-License-Identifier: AGPL-3.0-only
// What an author learns before posting on a server that allows a translation provider `private` or `direct`
// (ADR-0148 §6): a reader's request can send the text of such a post to that provider. `GET /instance` publishes which
// provider may receive what.
import { useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import type { Visibility } from "../api/compose";
import { visibilityLabel } from "./visibilityMessages";

/** The visibilities that a server keeps from its providers unless its operator allows them (ADR-0148 §5). */
const narrow: readonly Visibility[] = ["private", "direct"];

/** The sentence for the visibility picker, or null on a server where no provider may receive a narrow visibility. */
export function useTranslationDisclosure(): string | null {
  const api = useApi();
  const t = useT();
  const intl = useIntl();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  const providers = instance.data?.translation.providers ?? [];
  const [first, second] = narrow.filter((visibility) =>
    providers.some((provider) => provider.visibilities.includes(visibility)),
  );
  if (first === undefined) return null;
  const names = intl.formatList(
    providers
      .filter((provider) => provider.visibilities.some((visibility) => narrow.includes(visibility)))
      .map((provider) => provider.name),
    { type: "conjunction" },
  );
  return second === undefined
    ? t("client:compose.visibility.translation.one", { visibility: t(visibilityLabel[first]), providers: names })
    : t("client:compose.visibility.translation.both", {
        first: t(visibilityLabel[first]),
        second: t(visibilityLabel[second]),
        providers: names,
      });
}
