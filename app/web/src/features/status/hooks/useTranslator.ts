// SPDX-License-Identifier: AGPL-3.0-only
// The way to translate a post on this server for this viewer, if there is one (ADR-0148 §5, §6, §12, §13). It is the
// one place that chooses: the card, its messages, and its states work with any Translator. A provider that may receive
// the post comes first (ADR-0148 §12), and the browser, which sends nothing anywhere and so takes every visibility and
// needs no sign-in, takes over when the server refuses: a limit, a post or a language that no provider takes, or a
// provider that fails. The operator's order of the providers stays on the server, which also decides by the post's
// effective visibility; `Status.visibility` is the visibility the author chose, so the server can still refuse a post
// whose author is limited, and the browser then translates it.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError } from "../../../lib/errors";
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
  type TranslatedComment,
  translateOnServer,
  translateReportCommentOnServer,
  translateReportedOnServer,
  translationFailedType,
  translationLimitType,
  translationUnavailableType,
  translationUnsupportedType,
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
 * Whether `error` is the server refusing to translate, which leaves the text to the browser (ADR-0148 §12): a monthly
 * limit, a post or a language that no provider takes, a provider that gave no usable answer, or any other failure of the
 * server. A post that the reader may not see, or a reader who may not ask, is not: the browser takes over neither.
 */
function refusedByServer(error: unknown): boolean {
  if (!(error instanceof ApiError)) return false;
  switch (error.type) {
    case translationLimitType:
    case translationUnavailableType:
    case translationUnsupportedType:
    case translationFailedType:
      return true;
  }
  return error.status >= 500;
}

type Way<T> = (targetLanguage: string, onDownload: (loaded: number) => void) => Promise<T>;

/**
 * The server's way with the browser's behind it: the browser translates what the server refuses. Once the server refused
 * the text of `key`, the next press goes to the browser directly, since a model to download needs the reader's press,
 * which the wait for the server may have used up.
 */
function useServerFirst(key: string) {
  const [refusedKey, setRefusedKey] = useState<string | null>(null);
  return <T>(server: Way<T>, browser: Way<T> | null): Way<T> => {
    if (browser === null) return server;
    if (refusedKey === key) return browser;
    return async (targetLanguage, onDownload) => {
      try {
        return await server(targetLanguage, onDownload);
      } catch (error) {
        if (!refusedByServer(error)) throw error;
        setRefusedKey(key);
        return browser(targetLanguage, onDownload);
      }
    };
  };
}

/**
 * The translator of `post` into `target`, or null when nothing can translate it. A provider needs a signed-in reader,
 * since the server counts what it costs against the user (ADR-0148 §7), and must be allowed the post's visibility; the
 * browser needs the operator's consent, the API, and a pair of languages that it does not report `unavailable`. With
 * both, the server comes first and the browser takes over when it refuses. On a moderator's page about `report`, a
 * report that points at the post, a provider receives it only while the operator lets the text of reports go to the
 * providers, and still only by its visibility (ADR-0148 §13).
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
  const inBrowser = useBrowserTranslates(true, post.language, target);
  const serverFirst = useServerFirst(JSON.stringify([post.id, post.edited_at ?? null, target, report ?? null]));
  const browser: Translator | null = inBrowser
    ? (targetLanguage, onDownload) => translateInBrowser(post, targetLanguage, onDownload)
    : null;
  if (!onServer) return browser;
  const server: Translator =
    report === undefined
      ? (targetLanguage) => translateOnServer(api, post.id, targetLanguage)
      : (targetLanguage) => translateReportedOnServer(api, report, post.id, targetLanguage);
  return serverFirst(server, browser);
}

/**
 * The translator of `comment`, the comment of `report`, for a moderator, or null when nothing can translate it. Only the
 * reporter and the moderators read a comment, so a provider receives it as a `private` post: only one allowed those,
 * and only while the operator lets the text of reports go to the providers (ADR-0148 §13). The browser, whose detector
 * finds the comment's language, since nobody names it, takes over when the server refuses, as for a post.
 */
export function useReportCommentTranslator(report: string, comment: string, target: string): CommentTranslator | null {
  const api = useApi();
  const viewer = useViewer();
  const translation = useInstanceTranslation();
  const onServer =
    viewer.status === "signed-in" &&
    translation?.reports_to_server === true &&
    translation.providers.some((provider) => provider.visibilities.includes("private"));
  const inBrowser = useBrowserTranslates(comment !== "", "und", target);
  const serverFirst = useServerFirst(JSON.stringify([report, comment, target]));
  if (comment === "") return null;
  const browser: CommentTranslator | null = inBrowser
    ? (targetLanguage, onDownload) => translateTextInBrowser(comment, targetLanguage, onDownload)
    : null;
  if (!onServer) return browser;
  return serverFirst<TranslatedComment>(
    (targetLanguage) => translateReportCommentOnServer(api, report, targetLanguage),
    browser,
  );
}
