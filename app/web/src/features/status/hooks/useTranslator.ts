// SPDX-License-Identifier: AGPL-3.0-only
// The way to translate a post on this server for this viewer, if there is one (ADR-0148 §5, §6, §12, §13). It is the
// one place that chooses: the card, its messages, and its states work with any Translator. A provider that may receive
// the post comes first (ADR-0148 §6), and otherwise the browser, which sends nothing anywhere and so takes every
// visibility and needs no sign-in. The operator's order of the providers stays on the server, which also decides by the
// post's effective visibility; `Status.visibility` is the visibility the author chose, so the server can still refuse a
// post whose author is limited.
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { useViewer } from "../../session";
import type { Status } from "../api/status";
import {
  browserTranslationQueryOptions,
  hasBrowserTranslator,
  translateInBrowser,
  translateTextInBrowser,
} from "../api/browserTranslation";
import {
  type CommentTranslator,
  isInLanguage,
  translateOnServer,
  translateReportCommentOnServer,
  translateReportedOnServer,
  type Translator,
} from "../api/translation";

/** What the server says it offers for translation, from GET /instance, which the shell reads on every page. */
function useInstanceTranslation() {
  const api = useApi();
  // The shell handles the failures of GET /instance; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  return instance.data?.translation;
}

/** Whether the browser may translate text in `language` into `target`: the operator's consent, the API, the pair. */
function useBrowserTranslates(asked: boolean, language: string, target: string): boolean {
  const translation = useInstanceTranslation();
  const inBrowser = useQuery({
    ...browserTranslationQueryOptions(language, target),
    enabled: asked && translation?.browser === true && hasBrowserTranslator() && !isInLanguage(language, target),
  });
  return inBrowser.data === true;
}

/**
 * The translator of `post` into `target`, or null when nothing can translate it. A provider needs a signed-in reader,
 * since the server counts what it costs against the user (ADR-0148 §7), and must be allowed the post's visibility; the
 * browser needs the operator's consent, the API, and a pair of languages that it does not report `unavailable`. On a
 * moderator's page about `report`, a report that points at the post, a provider receives it only while the operator
 * lets the text of reports go to the providers, and still only by its visibility (ADR-0148 §13).
 */
export function useTranslator(post: Status, target: string, report?: string): Translator | null {
  const api = useApi();
  const viewer = useViewer();
  const translation = useInstanceTranslation();
  const providers = translation?.providers ?? [];
  const onServer =
    viewer.status === "signed-in" &&
    (report === undefined || translation?.reports_to_server === true) &&
    providers.some((provider) => provider.visibilities.includes(post.visibility));
  const inBrowser = useBrowserTranslates(!onServer, post.language, target);
  if (onServer) {
    return report === undefined
      ? (targetLanguage) => translateOnServer(api, post.id, targetLanguage)
      : (targetLanguage) => translateReportedOnServer(api, report, post.id, targetLanguage);
  }
  if (inBrowser) return (targetLanguage, onDownload) => translateInBrowser(post, targetLanguage, onDownload);
  return null;
}

/**
 * The translator of `comment`, the comment of `report`, for a moderator, or null when nothing can translate it. Only the
 * reporter and the moderators read a comment, so a provider receives it as a `private` post: only one allowed those,
 * and only while the operator lets the text of reports go to the providers (ADR-0148 §13). Otherwise the browser, whose
 * detector finds the comment's language, since nobody names it.
 */
export function useReportCommentTranslator(report: string, comment: string, target: string): CommentTranslator | null {
  const api = useApi();
  const viewer = useViewer();
  const translation = useInstanceTranslation();
  const onServer =
    viewer.status === "signed-in" &&
    translation?.reports_to_server === true &&
    translation.providers.some((provider) => provider.visibilities.includes("private"));
  const inBrowser = useBrowserTranslates(!onServer && comment !== "", "und", target);
  if (comment === "") return null;
  if (onServer) return (targetLanguage) => translateReportCommentOnServer(api, report, targetLanguage);
  if (inBrowser) return (targetLanguage, onDownload) => translateTextInBrowser(comment, targetLanguage, onDownload);
  return null;
}
