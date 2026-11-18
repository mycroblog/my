// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's own profile and how it is found (FR-006, FR-213, FR-307, FR-311, FR-412, FR-913): editing it, with an
// avatar and a header image uploaded first; the role badges they show (ADR-0070 §12); the hashtags they feature; and the
// hashtags they follow. A write puts what the server answered into the cache: the viewer's own account, and the copy
// of it the profile shows, and marks the places that carry the account's name and picture.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery, type QueryClient } from "@tanstack/react-query";
import {
  accountByIdQuery,
  featuredHashtagsQuery,
  markAccountLists,
  viewerQuery,
  type FeaturedHashtag,
} from "./accounts";
import type { AppData } from "./app-data";
import { refusalDetail, refusedField, uploadAttachment } from "./compose";
import { inOrder } from "./in-order";
import { wholeList, type PagedList } from "./paged";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";

/** The viewer's own account: the profile with the choices only its owner reads. */
export type CredentialAccount = components["schemas"]["CredentialAccount"];

/** The fields of the profile the server takes, as `PATCH /accounts/update_credentials` names them. */
export type CredentialsRequest = components["schemas"]["UpdateCredentialsRequest"];

/** A field of the profile (a label and its value as plain text); the profile has at most 4. */
export type ProfileField = components["schemas"]["AccountFieldSource"];

/** Who may read the lists of whom the viewer follows and who follows them (FR-311). */
export type FollowsVisibility = components["schemas"]["FollowsVisibility"];

/**
 * A change of the profile: the fields to change, the others stay. `avatar` and `header` are a picked image, uploaded
 * first (and waited for until the server has processed it), or null to remove the picture.
 */
export type ProfileChange = Omit<CredentialsRequest, "avatar_media_id" | "header_media_id"> & {
  readonly avatar?: File | null;
  readonly header?: File | null;
};

/** The most characters of a display name, of the profile text, and of a field's label and value (code points). */
export const profileLimits = { displayName: 30, note: 500, fieldText: 255, fields: 4, featuredHashtags: 10 } as const;

/** A picture of the profile. */
export type ProfilePicture = "avatar" | "header";

/**
 * The document the writes of the viewer's account are one of (`updateProfile`, `chooseBadge`), each sent once the
 * ones before it were answered.
 */
const credentialsWrites = "/accounts/update_credentials";

/** The picture each failed upload of a profile change was of, by the error the change rejected with. */
const uploadsFailed = new WeakMap<object, ProfilePicture>();

/**
 * The media each picked file was uploaded as, by session, until a change that carries it is saved: a change that
 * was not saved (the other picture's upload failed, the server refused the change) is sent again with the files
 * as they are, and a file already uploaded is not uploaded a second time. A file picked anew is another file.
 */
const uploaded = new WeakMap<QueryClient, WeakMap<File, string>>();

/** The uploads kept of the session of `data`. */
function uploadsOf({ queryClient }: AppData): WeakMap<File, string> {
  let kept = uploaded.get(queryClient);
  if (kept === undefined) {
    kept = new WeakMap();
    uploaded.set(queryClient, kept);
  }
  return kept;
}

/** The media of a picture that is no file to upload: empty to remove it, undefined to leave it. */
const unpicked = (file: null | undefined): string | undefined => (file === null ? "" : undefined);

/**
 * The media of a picture to set: its ID once uploaded (the one it was uploaded as, for a file a change that was not
 * saved had uploaded), empty to remove it, undefined to leave it.
 */
async function pictureId(
  data: AppData,
  picture: ProfilePicture,
  file: File | null | undefined,
): Promise<string | undefined> {
  if (file === undefined || file === null) return unpicked(file);
  const kept = uploadsOf(data);
  const had = kept.get(file);
  if (had !== undefined) return had;
  try {
    const { id } = await uploadAttachment(data, file);
    kept.set(file, id);
    return id;
  } catch (error) {
    if (typeof error === "object" && error !== null) uploadsFailed.set(error, picture);
    throw error;
  }
}

/**
 * The picture a rejection of `updateProfile` is about: the one whose upload failed (the error is the upload's own:
 * the server's problem, MediaFailedError, or mykit's), or the one the server refused in the change itself (a 422 that
 * names `avatar_media_id` or `header_media_id`). Undefined for a failure that is about no picture.
 */
export function failedPicture(error: unknown): ProfilePicture | undefined {
  if (typeof error !== "object" || error === null) return undefined;
  const uploaded = uploadsFailed.get(error);
  if (uploaded !== undefined) return uploaded;
  const member = refusedField(error)?.member;
  return member === "avatar_media_id" ? "avatar" : member === "header_media_id" ? "header" : undefined;
}

/**
 * The types of the problems the server refuses a file with as it is uploaded, whose title says what is wrong with the
 * file and what to choose instead, with no detail (internal/interface/clientapi/media.go).
 */
const refusedUploads: ReadonlySet<string> = new Set(
  ["media-too-large", "unsupported-media-format", "media-dimensions", "media-corrupt", "media-too-long"].map(
    (kind) => `/api/v1/problems/${kind}`,
  ),
);

/**
 * What the server said of the picture a rejection of `updateProfile` is about (`failedPicture`), in the reader's
 * language: the problem's detail (an image the account cannot use), else, for a file it refused as it was uploaded
 * (too large, a format it does not take, too many pixels, one it could not read), the problem's title. Undefined where
 * the server gave no words about the picture (a bare status, the network, a file it could not process), which the
 * caller words.
 */
export function pictureRefusal(error: unknown): string | undefined {
  if (!(error instanceof ProblemError)) return undefined;
  return refusalDetail(error) ?? (refusedUploads.has(error.type) ? error.title : undefined);
}

/**
 * Changes the viewer's profile, and resolves with the account the server answered once the cache holds it: as the
 * viewer's own, and as the profile of the account; the posts, notifications and lists that carry the account are read
 * again when they open. It rejects with the server's problem, or with the error of an upload that failed, and
 * nothing is changed then; `failedPicture` says which picture such an error is about.
 *
 * Every answer is the whole account, so the changes are sent one at a time, in the order made, each once the ones
 * before it were answered (`inOrder`, with `chooseBadge`, which reads the account again): two on their way at once
 * could be answered crossed, the older account put back over the newer. A change that uploads nothing takes its
 * place in that order as it is made; one with a picture, once its pictures are uploaded, outside that order, each
 * file once: a change made again after it failed sends the media its files were uploaded as, but for a picture the
 * server refused in the change itself, which is uploaded anew.
 */
export function updateProfile(data: AppData, change: ProfileChange): Promise<CredentialAccount> {
  const { avatar, header, ...fields } = change;
  const files = [avatar, header].filter((file): file is File => file instanceof File);
  const kept = uploadsOf(data);
  const write = (avatarId: string | undefined, headerId: string | undefined) =>
    inOrder(data, credentialsWrites, async () => {
      const { data: saved } = await data.api
        .PATCH("/accounts/update_credentials", {
          body: {
            ...fields,
            ...(avatarId === undefined ? {} : { avatar_media_id: avatarId }),
            ...(headerId === undefined ? {} : { header_media_id: headerId }),
          },
        })
        .catch((error: unknown) => {
          // Media the server does not take as this picture is not sent again: a retry uploads the file anew.
          const refused = failedPicture(error);
          const file = refused === "avatar" ? avatar : refused === "header" ? header : undefined;
          if (file instanceof File) kept.delete(file);
          throw error;
        });
      if (saved === undefined) throw new Error("app-data: update_credentials answered without an account");
      // The media are the account's pictures now.
      for (const file of files) kept.delete(file);
      const { queryClient } = data;
      queryClient.setQueryData(viewerQuery(data).queryKey, saved);
      queryClient.setQueryData(accountByIdQuery(data, saved.id).queryKey, saved);
      await markAccountLists(queryClient, saved.id, "later");
      return saved;
    });
  if (!(avatar instanceof File) && !(header instanceof File)) return write(unpicked(avatar), unpicked(header));
  return (async () => write(await pictureId(data, "avatar", avatar), await pictureId(data, "header", header)))();
}

/** Changes the viewer's profile, from a screen (`updateProfile`). */
export function useUpdateProfile(): (change: ProfileChange) => Promise<CredentialAccount> {
  const data = useAppData();
  return (change) => updateProfile(data, change);
}

/** The viewer's own account, with the choices only its owner reads; undefined until the server answered. */
export function useCredentials(): CredentialAccount | undefined {
  return useQuery(viewerQuery(useAppData())).data;
}

/**
 * Where the reads of what a screen of the profile's settings shows are: the viewer's own account, and with `lists`
 * the badges they may show and the hashtags they feature too (not read without it). `loading` until the server
 * answered them all, `failed` when it did not answer one, with `retry`, which reads again those that failed. A list
 * that is `read` is the server's, not the empty one of a read still on its way, so a change of it replaces what the
 * server holds and nothing else. What was read stays `read`: a later read of it that fails leaves the screen what it
 * had (`readOf`).
 */
export function useProfileRead({ lists = false }: { lists?: boolean } = {}): ScreenRead {
  const data = useAppData();
  const account = useQuery(viewerQuery(data));
  const badges = useQuery({ ...badgeChoicesQuery(data), enabled: lists });
  const featured = useQuery({ ...ownFeaturedHashtagsQuery(data), enabled: lists });
  return readOf(lists ? [account, badges, featured] : [account]);
}

// ---- Badges --------------------------------------------------------------------------------------------------------

/** A role badge the viewer may show on their profile, and whether they do. */
export type BadgeChoice = components["schemas"]["BadgeChoice"];

/** The badges of the roles the viewer holds, with the choice to show each. */
export function badgeChoicesQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences/badges");
}

/** The viewer's badge choices; empty until the server answered. */
export function useBadgeChoices(): readonly BadgeChoice[] {
  return useQuery(badgeChoicesQuery(useAppData())).data?.items ?? [];
}

/**
 * Shows the badge of the role of `roleId` on the viewer's profile, or stops showing it, and resolves once the cache
 * holds the choice as saved and the viewer's account, which carries the badges they show, is read again. It rejects
 * with the server's problem (404: a role the viewer does not hold, or without a badge). It is one of the writes of
 * the account (`updateProfile`), sent and read again in their order: its read of the account, on its way while a
 * later change of the profile is answered, would put the older account back.
 */
export function chooseBadge(data: AppData, roleId: string, displayed: boolean): Promise<void> {
  return inOrder(data, credentialsWrites, async () => {
    const { data: saved } = await data.api.PUT("/preferences/badges/{role_id}", {
      params: { path: { role_id: roleId } },
      body: { displayed },
    });
    if (saved !== undefined)
      data.queryClient.setQueryData<components["schemas"]["BadgeChoiceList"]>(
        badgeChoicesQuery(data).queryKey,
        (now) =>
          now === undefined ? now : { ...now, items: now.items.map((c) => (c.role_id === saved.role_id ? saved : c)) },
      );
    await data.queryClient.invalidateQueries({ queryKey: viewerQuery(data).queryKey });
  });
}

/** Shows or hides a role badge, from a screen (`chooseBadge`). */
export function useChooseBadge(): (roleId: string, displayed: boolean) => Promise<void> {
  const data = useAppData();
  return (roleId, displayed) => chooseBadge(data, roleId, displayed);
}

// ---- Featured hashtags ---------------------------------------------------------------------------------------------

/** The viewer's own featured hashtags, in the order the server lists them. */
export function ownFeaturedHashtagsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/featured_tags");
}

/** The viewer's featured hashtags; empty until the server answered. */
export function useOwnFeaturedHashtags(): readonly FeaturedHashtag[] {
  return useQuery(ownFeaturedHashtagsQuery(useAppData())).data?.items ?? [];
}

/**
 * Features exactly the hashtags of `names` (with or without `#`, at most 10, an empty list features none), and
 * resolves once the cache holds the list as saved, here and as the profile shows it. Each list is sent once the ones
 * set before it were answered, so that the last one set is the one the server and the cache keep. It rejects with the
 * server's problem.
 */
export function setFeaturedHashtags(data: AppData, names: readonly string[]): Promise<void> {
  return inOrder(data, "/featured_tags", async () => {
    const { data: saved } = await data.api.PUT("/featured_tags", { body: { names: [...names] } });
    if (saved === undefined) return;
    const { queryClient } = data;
    queryClient.setQueryData(ownFeaturedHashtagsQuery(data).queryKey, saved);
    const id = queryClient.getQueryData<CredentialAccount>(viewerQuery(data).queryKey)?.id;
    if (id !== undefined) queryClient.setQueryData(featuredHashtagsQuery(data, id).queryKey, saved);
  });
}

/** Sets the featured hashtags, from a screen (`setFeaturedHashtags`). */
export function useSetFeaturedHashtags(): (names: readonly string[]) => Promise<void> {
  const data = useAppData();
  return (names) => setFeaturedHashtags(data, names);
}

// ---- Followed hashtags ---------------------------------------------------------------------------------------------

/** A hashtag the viewer follows, and when they started. */
export type FollowedHashtag = components["schemas"]["FollowedHashtag"];

/** The hashtags the viewer follows. */
export function followedHashtagsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/followed_tags");
}

/** The hashtags the viewer follows; empty until the server answered. */
export function useFollowedHashtags(): readonly FollowedHashtag[] {
  return useQuery(followedHashtagsQuery(useAppData())).data?.items ?? [];
}

/** The hashtags the viewer follows, the latest followed first, for a screen that lists them with how its read went. */
export function useFollowedHashtagList(): PagedList<FollowedHashtag> {
  const query = useQuery(followedHashtagsQuery(useAppData()));
  return wholeList({ ...query, data: query.data?.items });
}
