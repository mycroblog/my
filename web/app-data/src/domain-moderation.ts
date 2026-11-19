// SPDX-License-Identifier: AGPL-3.0-only
// The domains as a holder of `manage_domains` moderates them (FR-702, FR-706, FR-716–FR-718, ADR-0159): the actions on
// remote domains, newest first, of one domain and in effect or all, a page at a time by the server's cursor; one action,
// and issuing and lifting one; what Mastodon's domain block CSV would do before anything is imported, importing the rows
// kept and exporting the actions in effect as that CSV; the allowlist of federation, read whole, and adding and
// removing a domain of it; and the subscriptions to the domain block lists others publish, with their fetches, their
// drafts, and accepting and rejecting those. Every write reads again what it changes: the lists, the thing written,
// and the counts of what awaits the moderators.
import { type components } from "@mycroblog/mykit";
import { keepPreviousData, skipToken, useQueries, useQuery } from "@tanstack/react-query";
import { whileOpen, type AppData } from "./app-data";
import type { SavedFile } from "./exports";
import { useAppData } from "./provider";
import { oneOf, pageOf, staffChanged, type StaffOne, type StaffPage } from "./staff-reads";

/** A moderation action on a remote domain and its subdomains. */
export type DomainAction = components["schemas"]["AdminDomainAction"];
/** What a domain action does: `suspend`, `limit`, `reject_media`, `reject_reports`, `force_sensitive`. */
export type DomainActionKind = components["schemas"]["AdminDomainActionKind"];
/** A new domain action, as the server takes it. */
export type DomainActionRequest = components["schemas"]["CreateDomainActionRequest"];
/** What a suspend of a domain would end now: the follows with its accounts, and the local accounts that hold one. */
export type DomainSuspendImpact = components["schemas"]["DomainSuspendImpact"];
/** What importing each row of a domain block CSV would do, in row order. */
export type DomainImportPreview = components["schemas"]["DomainBlockImportPreview"];
/** A row of a domain block CSV and what importing it would do. */
export type DomainImportRow = components["schemas"]["DomainBlockImportRow"];
/** What importing a row would do (`importable`), or why it would do nothing. */
export type DomainImportStatus = components["schemas"]["DomainBlockImportStatus"];
/** A row of Mastodon's domain block CSV, as the import takes it. */
export type DomainBlock = components["schemas"]["DomainBlock"];
/** What an import issued and skipped. */
export type DomainImportResult = components["schemas"]["DomainBlockImportResult"];
/** A domain of the allowlist of federation. */
export type DomainAllow = components["schemas"]["DomainAllow"];
/** A subscription to a domain block list. */
export type DomainListSubscription = components["schemas"]["AdminDomainListSubscription"];
/** A new subscription, as the server takes it. */
export type DomainListSubscriptionRequest = components["schemas"]["CreateDomainListSubscriptionRequest"];
/** The changes to a subscription's settings. */
export type DomainListSubscriptionChange = components["schemas"]["UpdateDomainListSubscriptionRequest"];
/** One fetch of a subscribed list. */
export type DomainListFetch = components["schemas"]["AdminDomainListFetch"];
/** A difference between a subscribed list and its actions, waiting for a moderator. */
export type DomainListDraft = components["schemas"]["AdminDomainListDraft"];
/** What accepting or rejecting drafts did. */
export type DomainListDraftResult = components["schemas"]["AdminDomainListDraftResult"];
/** The format of a subscribed list. */
export type DomainListFormat = components["schemas"]["AdminDomainListFormat"];
/** Whether a list's differences wait for a moderator (`draft`) or are applied at each fetch (`automatic`). */
export type DomainListMode = components["schemas"]["AdminDomainListMode"];
/** What a list's `suspend` issues here. */
export type DomainListSuspendAs = components["schemas"]["AdminDomainListSuspendAs"];
/** What a list's `silence` issues here. */
export type DomainListSilenceAs = components["schemas"]["AdminDomainListSilenceAs"];

/** The kinds of a domain action, the gravest first. */
export const domainActionKinds = [
  "suspend",
  "limit",
  "reject_media",
  "reject_reports",
  "force_sensitive",
] as const satisfies readonly DomainActionKind[];
/** The formats of a subscribed list, in the server's order. */
export const domainListFormats = [
  "mastodon_csv",
  "mastodon_json",
  "plain_text",
] as const satisfies readonly DomainListFormat[];
export const domainListModes = ["draft", "automatic"] as const satisfies readonly DomainListMode[];
export const domainListSuspendAs = ["suspend", "limit", "ignore"] as const satisfies readonly DomainListSuspendAs[];
export const domainListSilenceAs = ["limit", "ignore"] as const satisfies readonly DomainListSilenceAs[];

/** The most characters of a domain, of an action's public comment and private note, and of an allow's comment. */
export const domainLimit = 253;
export const domainCommentLimit = 5000;
/** The longest an action may last, in seconds: ten years. */
export const domainActionLongest = 315_360_000;
/** The most rows an import takes. */
export const domainImportRowLimit = 5000;
/** The most characters of a subscription's title and URL. */
export const domainListTitleLimit = 200;
export const domainListUrlLimit = 2048;

/** What the list of domain actions shows: the actions on one domain, those in effect alone, and the page after `cursor`. */
export type DomainActionQuery = {
  readonly domain?: string | undefined;
  readonly active?: boolean | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the domain actions `q` asks for. */
export function domainActionsQuery({ queries }: AppData, q: DomainActionQuery) {
  return queries.queryOptions("get", "/admin/domain_actions", {
    params: {
      query: {
        ...(q.domain === undefined ? {} : { domain: q.domain }),
        ...(q.active === true ? { active: true } : {}),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** The page of domain actions `q` asks for; the page shown before stays while another is on its way (`stale`). */
export function useDomainActions(q: DomainActionQuery): StaffPage<DomainAction> {
  return pageOf(useQuery({ ...domainActionsQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** Reads the page of domain actions `q` asks for, for a route's loader. */
export async function loadDomainActions(data: AppData, q: DomainActionQuery): Promise<void> {
  await data.queryClient.query(domainActionsQuery(data, q)).catch(() => undefined);
}

/** The domain action of `id`. */
export function domainActionQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/domain_actions/{id}", { params: { path: { id } } });
}

/** The domain action of `id`, as a screen shows it. */
export function useDomainAction(id: string): StaffOne<DomainAction> {
  return oneOf(useQuery(domainActionQuery(useAppData(), id)));
}

/** Reads the domain action of `id`, for a route's loader. */
export async function loadDomainAction(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(domainActionQuery(data, id)).catch(() => undefined);
}

/** What the domain actions change, read again after a write: their lists and the actions, and the audit log. */
const domainActionsChanged = (data: AppData) => {
  staffChanged(data, ["/admin/domain_actions", "/admin/audit_log"]);
};

/** Puts the action a write answered into the cache, and reads again what it changes. */
async function actionWritten(data: AppData, action: DomainAction | undefined): Promise<DomainAction> {
  if (action === undefined) throw new Error("app-data: a write of a domain action answered without the action");
  const key = domainActionQuery(data, action.id).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  data.queryClient.setQueryData(key, action);
  domainActionsChanged(data);
  return action;
}

/** What a holder of `manage_domains` does to the domain actions; each rejects with the server's problem. */
export type DomainActionWrites = {
  /** Issues an action; a 422 names `domain` (not a domain, or this server's), `public_comment` or `private_note`. */
  readonly issue: (request: DomainActionRequest) => Promise<DomainAction>;
  /** Lifts the action of `id`; one lifted already stays as it is. */
  readonly lift: (id: string) => Promise<DomainAction>;
  /** Imports the rows `blocks` of a CSV, each action with `privateNote`, as the preview said they would. */
  readonly importBlocks: (blocks: readonly DomainBlock[], privateNote: string) => Promise<DomainImportResult>;
  /** The actions in effect as Mastodon's domain block CSV, to save; it never settles once the session ended. */
  readonly exportCsv: () => Promise<SavedFile>;
};

/** The name in a Content-Disposition header, else `fallback`. */
function fileName(disposition: string | null, fallback: string): string {
  const name = /filename="?([^";]+)"?/.exec(disposition ?? "")?.[1];
  return name === undefined || name === "" ? fallback : name;
}

/** What a holder of `manage_domains` does to the domain actions, as `DomainActionWrites`. */
export function domainActionWrites(data: AppData): DomainActionWrites {
  return {
    issue: async (request) =>
      actionWritten(data, (await data.api.POST("/admin/domain_actions", { body: request })).data),
    lift: async (id) =>
      actionWritten(data, (await data.api.POST("/admin/domain_actions/{id}/lift", { params: { path: { id } } })).data),
    importBlocks: async (blocks, privateNote) => {
      const { data: result } = await data.api.POST("/admin/domain_actions/import", {
        body: { blocks: [...blocks], ...(privateNote.trim() === "" ? {} : { private_note: privateNote.trim() }) },
      });
      if (result === undefined) throw new Error("app-data: the import answered without its result");
      domainActionsChanged(data);
      return result;
    },
    exportCsv: () =>
      whileOpen(data, async () => {
        const { data: blob, response } = await data.api.GET("/admin/domain_actions/export", { parseAs: "blob" });
        return {
          blob: blob ?? new Blob([]),
          name: fileName(response.headers.get("Content-Disposition"), "domain_blocks.csv"),
        };
      }),
  };
}

/** What a holder of `manage_domains` does to the domain actions, for a screen (`domainActionWrites`). */
export function useDomainActionWrites(): DomainActionWrites {
  return domainActionWrites(useAppData());
}

/** What a suspend of `domain` would end now. */
export function domainSuspendImpactQuery({ queries }: AppData, domain: string) {
  return queries.queryOptions("get", "/admin/domain_actions/impact", { params: { query: { domain } } });
}

/**
 * What a suspend of `domain` would end now, as the confirmation before it shows it: read as the confirmation opens and
 * not kept once it closes, since the follows change; a 422 for a domain that is not one, or this server's.
 */
export function useDomainSuspendImpact(domain: string): StaffOne<DomainSuspendImpact> {
  return oneOf(useQuery({ ...domainSuspendImpactQuery(useAppData(), domain), staleTime: 0, gcTime: 0, retry: false }));
}

/** The most domains whose suspends a confirmation counts the follows of; past them, it says they are uncounted. */
export const domainImpactCountLimit = 20;

/** What the suspends of several domains would end, summed, and where its read is. */
export type DomainSuspendImpacts = {
  readonly follows: number;
  readonly localAccounts: number;
  /** `pending` while any count is on its way; `error` when one failed or there are too many domains to count. */
  readonly status: "pending" | "error" | "success";
};

/**
 * What the suspends of `domains` would end now, each counted as `useDomainSuspendImpact` counts it and summed (a
 * person who loses follows on two domains counted on each); past `domainImpactCountLimit` domains nothing is asked.
 */
export function useDomainSuspendImpacts(domains: readonly string[]): DomainSuspendImpacts {
  const data = useAppData();
  const counted = domains.length <= domainImpactCountLimit;
  const reads = useQueries({
    queries: (counted ? domains : []).map((domain) => ({
      ...domainSuspendImpactQuery(data, domain),
      staleTime: 0,
      gcTime: 0,
      retry: false,
    })),
  });
  if (!counted || reads.some((r) => r.status === "error")) return { follows: 0, localAccounts: 0, status: "error" };
  if (reads.some((r) => r.status === "pending")) return { follows: 0, localAccounts: 0, status: "pending" };
  return {
    follows: reads.reduce((n, r) => n + (r.data?.follows ?? 0), 0),
    localAccounts: reads.reduce((n, r) => n + (r.data?.local_accounts ?? 0), 0),
    status: "success",
  };
}

/** The preview of a CSV as a screen follows it. */
export type DomainImportPreviewState = {
  /** What importing the CSV given would do; undefined until the server answered for it. */
  readonly preview: DomainImportPreview | undefined;
  /** Where the preview is: `idle` with no CSV to ask about. */
  readonly status: "idle" | "pending" | "error" | "success";
  /** The server's problem, when it refused the file (a 422 naming `csv`, a 413 for a body too large). */
  readonly error: unknown;
};

/**
 * What importing `csv` would do, asked whenever it changes; nothing is asked without one, and nothing changes on the
 * server. The answer is not kept once no screen shows it: the file's text is its key.
 */
export function useDomainImportPreview(csv: string | undefined): DomainImportPreviewState {
  const data = useAppData();
  const query = useQuery({
    // A key of its own, outside the domain actions' paths, which a write reads again: the preview is no list of them,
    // and asking for it again sends the whole file.
    queryKey: ["domain-import-preview", csv] as const,
    queryFn:
      csv === undefined
        ? skipToken
        : async ({ signal }) => {
            const { data: preview } = await data.api.POST("/admin/domain_actions/import/preview", {
              body: { csv },
              signal,
            });
            if (preview === undefined) throw new Error("app-data: the preview answered without its rows");
            return preview;
          },
    staleTime: Number.POSITIVE_INFINITY,
    gcTime: 0,
    retry: false,
  });
  return {
    preview: csv === undefined ? undefined : query.data,
    status: csv === undefined ? "idle" : query.status,
    error: csv === undefined ? null : query.error,
  };
}

/** The allowlist of federation, whole (the server lists at most 5,000 domains, unpaged). */
export function domainAllowsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/domain_allows");
}

/** The allowlist of federation, as a screen shows it. */
export function useDomainAllows(): StaffOne<readonly DomainAllow[]> {
  const read = oneOf(useQuery(domainAllowsQuery(useAppData())));
  return { ...read, value: read.value?.items };
}

/** Reads the allowlist, for a route's loader. */
export async function loadDomainAllows(data: AppData): Promise<void> {
  await data.queryClient.query(domainAllowsQuery(data)).catch(() => undefined);
}

/** What a holder of `manage_domains` does to the allowlist; each rejects with the server's problem. */
export type DomainAllowWrites = {
  /**
   * Allows `domain` and its subdomains, with a comment nobody outside the staff reads; one allowed already keeps its
   * allow. A 400 is a domain that is not a host; a 422 names `domain` (this server's), `comment` or `domains` (more
   * than 5,000).
   */
  readonly add: (domain: string, comment: string) => Promise<DomainAllow>;
  /** Removes the allow of `domain`; one without an allow changes nothing. */
  readonly remove: (domain: string) => Promise<void>;
};

/** What a holder of `manage_domains` does to the allowlist, as `DomainAllowWrites`. */
export function domainAllowWrites(data: AppData): DomainAllowWrites {
  const changed = () => {
    staffChanged(data, ["/admin/domain_allows", "/admin/audit_log"]);
  };
  return {
    add: async (domain, comment) => {
      const { data: allow } = await data.api.POST("/admin/domain_allows", {
        body: { domain, ...(comment.trim() === "" ? {} : { comment: comment.trim() }) },
      });
      if (allow === undefined) throw new Error("app-data: an allow answered without the allow");
      changed();
      return allow;
    },
    remove: async (domain) => {
      await data.api.DELETE("/admin/domain_allows/{domain}", { params: { path: { domain } } });
      changed();
    },
  };
}

/** What a holder of `manage_domains` does to the allowlist, for a screen (`domainAllowWrites`). */
export function useDomainAllowWrites(): DomainAllowWrites {
  return domainAllowWrites(useAppData());
}

/** A page of the subscriptions, newest first, after `cursor`. */
export function domainListSubscriptionsQuery({ queries }: AppData, cursor: string | undefined) {
  return queries.queryOptions("get", "/admin/domain_list_subscriptions", {
    params: { query: cursor === undefined ? {} : { cursor } },
  });
}

/** The page of subscriptions after `cursor`, as a screen shows it. */
export function useDomainListSubscriptions(cursor: string | undefined): StaffPage<DomainListSubscription> {
  return pageOf(useQuery({ ...domainListSubscriptionsQuery(useAppData(), cursor), placeholderData: keepPreviousData }));
}

/** Reads the page of subscriptions after `cursor`, for a route's loader. */
export async function loadDomainListSubscriptions(data: AppData, cursor: string | undefined): Promise<void> {
  await data.queryClient.query(domainListSubscriptionsQuery(data, cursor)).catch(() => undefined);
}

/** The subscription of `id`. */
export function domainListSubscriptionQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/domain_list_subscriptions/{id}", { params: { path: { id } } });
}

/** The subscription of `id`, as a screen shows it. */
export function useDomainListSubscription(id: string): StaffOne<DomainListSubscription> {
  return oneOf(useQuery(domainListSubscriptionQuery(useAppData(), id)));
}

/** The drafts of the subscription of `id`, the lifts first. */
export function domainListDraftsQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/domain_list_subscriptions/{id}/drafts", { params: { path: { id } } });
}

/** The drafts of the subscription of `id`, as a screen shows them. */
export function useDomainListDrafts(id: string): StaffOne<readonly DomainListDraft[]> {
  const read = oneOf(useQuery(domainListDraftsQuery(useAppData(), id)));
  return { ...read, value: read.value?.items };
}

/** The fetches of the subscription of `id`, newest first, after `cursor`. */
export function domainListFetchesQuery({ queries }: AppData, id: string, cursor: string | undefined) {
  return queries.queryOptions("get", "/admin/domain_list_subscriptions/{id}/fetches", {
    params: { path: { id }, query: cursor === undefined ? {} : { cursor } },
  });
}

/** The page of fetches of the subscription of `id` after `cursor`, as a screen shows it. */
export function useDomainListFetches(id: string, cursor: string | undefined): StaffPage<DomainListFetch> {
  return pageOf(useQuery({ ...domainListFetchesQuery(useAppData(), id, cursor), placeholderData: keepPreviousData }));
}

/** Reads the subscription of `id`, its drafts and its latest fetches, for a route's loader. */
export async function loadDomainListSubscription(data: AppData, id: string): Promise<void> {
  await Promise.all([
    data.queryClient.query(domainListSubscriptionQuery(data, id)).catch(() => undefined),
    data.queryClient.query(domainListDraftsQuery(data, id)).catch(() => undefined),
    data.queryClient.query(domainListFetchesQuery(data, id, undefined)).catch(() => undefined),
  ]);
}

/** What a holder of `manage_domains` does to the subscriptions; each rejects with the server's problem. */
export type DomainListWrites = {
  /** Subscribes to a list, which the server fetches at once; a 422 is a URL that is not https or an invalid setting. */
  readonly subscribe: (request: DomainListSubscriptionRequest) => Promise<DomainListSubscription>;
  /** Changes the settings of the subscription of `id`, and queues a fetch that applies them. */
  readonly change: (id: string, change: DomainListSubscriptionChange) => Promise<DomainListSubscription>;
  /** Stops the subscription of `id`; its actions stay, unless `liftActions`, which lifts them first. */
  readonly remove: (id: string, liftActions: boolean) => Promise<void>;
  /** Queues a fetch of the list of `id` now. */
  readonly fetchNow: (id: string) => Promise<void>;
  /** Accepts the drafts of `keys` of the subscription of `id`, issuing or lifting their actions; every draft for none. */
  readonly accept: (id: string, keys: readonly string[] | undefined) => Promise<DomainListDraftResult>;
  /** Rejects the drafts of `keys` of the subscription of `id`; every draft for none. */
  readonly reject: (id: string, keys: readonly string[] | undefined) => Promise<DomainListDraftResult>;
};

/** What a holder of `manage_domains` does to the subscriptions, as `DomainListWrites`. */
export function domainListWrites(data: AppData): DomainListWrites {
  const changed = (actions: boolean) => {
    staffChanged(data, [
      "/admin/domain_list_subscriptions",
      "/admin/audit_log",
      ...(actions ? ["/admin/domain_actions"] : []),
    ]);
  };
  const written = async (subscription: DomainListSubscription | undefined): Promise<DomainListSubscription> => {
    if (subscription === undefined) throw new Error("app-data: a subscription's write answered without it");
    const key = domainListSubscriptionQuery(data, subscription.id).queryKey;
    await data.queryClient.cancelQueries({ queryKey: key });
    data.queryClient.setQueryData(key, subscription);
    changed(false);
    return subscription;
  };
  const decided = (result: DomainListDraftResult | undefined): DomainListDraftResult => {
    if (result === undefined) throw new Error("app-data: the drafts answered without their result");
    changed(true);
    return result;
  };
  const keysOf = (keys: readonly string[] | undefined) => (keys === undefined ? {} : { body: { keys: [...keys] } });
  return {
    subscribe: async (request) =>
      written((await data.api.POST("/admin/domain_list_subscriptions", { body: request })).data),
    change: async (id, change) =>
      written(
        (await data.api.PATCH("/admin/domain_list_subscriptions/{id}", { params: { path: { id } }, body: change }))
          .data,
      ),
    remove: async (id, liftActions) => {
      await data.api.DELETE("/admin/domain_list_subscriptions/{id}", {
        params: { path: { id }, query: liftActions ? { lift_actions: true } : {} },
      });
      changed(liftActions);
    },
    fetchNow: async (id) => {
      await data.api.POST("/admin/domain_list_subscriptions/{id}/fetch", { params: { path: { id } } });
      changed(false);
    },
    accept: async (id, keys) =>
      decided(
        (
          await data.api.POST("/admin/domain_list_subscriptions/{id}/drafts/accept", {
            params: { path: { id } },
            ...keysOf(keys),
          })
        ).data,
      ),
    reject: async (id, keys) =>
      decided(
        (
          await data.api.POST("/admin/domain_list_subscriptions/{id}/drafts/reject", {
            params: { path: { id } },
            ...keysOf(keys),
          })
        ).data,
      ),
  };
}

/** What a holder of `manage_domains` does to the subscriptions, for a screen (`domainListWrites`). */
export function useDomainListWrites(): DomainListWrites {
  return domainListWrites(useAppData());
}
