// SPDX-License-Identifier: AGPL-3.0-only
// Whether the viewer may write new articles (ADR-0160 §3): everyone may, unless the instance setting
// `article_authoring` is `permission`, which needs the permission `create_articles` of a role. It only hides the way in:
// the server checks createArticle (and the publish of an article's draft) itself.
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { credentialsQueryOptions, instanceQueryOptions } from "../api/compose";

/** Whether the signed-in viewer may write a new article; false while the instance or the account loads. */
export function useCanWriteArticles(): boolean {
  const api = useApi();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader }).data;
  const viewer = useQuery({ ...credentialsQueryOptions(api), ...shellQueryReader }).data;
  if (instance === undefined || viewer === undefined) return false;
  return instance.article_authoring !== "permission" || viewer.permissions.includes("create_articles");
}
