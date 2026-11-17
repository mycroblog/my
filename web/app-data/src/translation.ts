// SPDX-License-Identifier: AGPL-3.0-only
// A post translated by the server's providers on the reader's request (FR-120, ADR-0148 §7, §12): what the instance
// offers (its providers, each with the visibilities it may receive, and whether the browser's translator may be
// used), and the translation of a status into a language, kept in the cache by the status, its version and the
// language, so that asking again, from any screen, asks the server once. The cache lives in memory only, as every
// translation must (§12). A refusal rejects with a TranslationRefusal that says why, which decides whether the
// browser's translator may take over (§12): the server's limit, a visibility no provider may receive, a language no
// provider translates, or the providers failing may be; a post the reader may not read, or a request refused for its
// authorization, never is.
import { NetworkError, ProblemError, type components } from "@mycroblog/mykit";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** A post in another language, as the server answers it. */
export type StatusTranslation = components["schemas"]["StatusTranslation"];

/** What the instance offers for translating posts. */
export type InstanceTranslation = components["schemas"]["InstanceTranslation"];

/**
 * Why the server did not translate a post: over a monthly limit (`limit`), no provider may receive its visibility
 * (`visibility`), none translates its language (`language`), the providers did not answer or the server failed
 * (`failed`), or the reader may not ask (`denied`: a post they cannot read, or no right to).
 */
export type TranslationRefusalReason = "limit" | "visibility" | "language" | "failed" | "denied";

/** The server's refusal to translate a post, with why; `cause` is mykit's error. */
export class TranslationRefusal extends Error {
  override readonly name = "TranslationRefusal";
  readonly reason: TranslationRefusalReason;

  constructor(reason: TranslationRefusalReason, cause: unknown) {
    super(`app-data: the server did not translate the post (${reason})`, { cause });
    this.reason = reason;
  }
}

const problems = "/api/v1/problems/";

/** Why `error`, of a request to translate, means the server did not translate. */
export function translationRefusalOf(error: unknown): TranslationRefusalReason {
  if (error instanceof NetworkError) return "failed";
  if (!(error instanceof ProblemError)) return "failed";
  if (error.type === `${problems}translation-limit`) return "limit";
  if (error.type === `${problems}translation-unavailable`) return "visibility";
  if (error.type === `${problems}translation-unsupported-language`) return "language";
  if (error.status >= 500 || error.status === 429 || error.status === 408) return "failed";
  return "denied";
}

/**
 * Whether the server's providers may receive a post of `visibility`: the instance lists a provider for it (§5, §6).
 * The server may still refuse the post of an author whose posts it limits, which reach the providers as followers-only.
 */
export function serverTranslates(offer: InstanceTranslation | undefined, visibility: string): boolean {
  return offer?.providers.some((p) => p.visibilities.some((v) => v === visibility)) ?? false;
}

/** The cache's key of the translation of a status's version into a language. */
const translationKey = (id: string, version: string, language: string) =>
  ["status-translation", id, version, language] as const;

/**
 * The status of `id` translated into `language` by the server, from the cache when it was asked for before: its
 * `version` (when it was last edited, or posted) keys what the cache holds, so an edit asks again. Rejects with a
 * TranslationRefusal.
 */
export async function translateStatus(
  data: AppData,
  id: string,
  language: string,
  version = "",
): Promise<StatusTranslation> {
  try {
    return await data.queryClient.query({
      queryKey: translationKey(id, version, language),
      queryFn: async () => {
        const { data: answer } = await data.api.POST("/statuses/{id}/translation", {
          params: { path: { id } },
          body: { target_language: language },
        });
        if (answer === undefined) throw new Error("app-data: the translation answered without a body");
        return answer;
      },
      staleTime: Number.POSITIVE_INFINITY,
    });
  } catch (e) {
    throw new TranslationRefusal(translationRefusalOf(e), e);
  }
}

/** translateStatus for the session of the nearest AppDataProvider. */
export function useTranslateStatus(): (id: string, language: string, version?: string) => Promise<StatusTranslation> {
  const data = useAppData();
  return (id, language, version) => translateStatus(data, id, language, version);
}
