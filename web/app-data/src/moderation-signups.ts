// SPDX-License-Identifier: AGPL-3.0-only
// Who may sign up, as the moderators keep it (FR-011, FR-719, FR-721, ADR-0058, ADR-0152): the registrations that wait
// for approval, approved or rejected (`manage_accounts`); the blocks on sign-ups by email domain, email address or IP
// range, which refuse a sign-up or make it wait for approval, added one at a time or from a list, and lifted; and the
// invites of the staff and of the members, with who created each, made and revoked (`manage_invites`). None of them is
// offered on a one-person server, whose server answers them with a 404 of its own. Every write reads again what it
// changes and the counts of what awaits the moderators.
import type { components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { oneOf, pageOf, staffChanged, type StaffOne, type StaffPage } from "./staff-reads";
import { writeCached } from "./written";

/** A registration as the moderators review it. */
export type ModeratedRegistration = components["schemas"]["AdminRegistration"];
/** A block on sign-ups. */
export type SignupBlock = components["schemas"]["AdminSignupBlock"];
/** What a block on sign-ups matches. */
export type SignupBlockKind = components["schemas"]["AdminSignupBlockKind"];
/** What a block does to a sign-up it matches. */
export type SignupBlockSeverity = components["schemas"]["AdminSignupBlockSeverity"];
/** A new block on sign-ups. */
export type NewSignupBlock = components["schemas"]["CreateSignupBlockRequest"];
/** A list of values to block sign-ups by, all with the same settings. */
export type SignupBlockImport = components["schemas"]["ImportSignupBlocksRequest"];
/** What an import of blocks added and skipped. */
export type SignupBlockImportResult = components["schemas"]["SignupBlockImportResult"];
/** An invite, the staff's or a member's, as its moderators list it. */
export type ModeratedInvite = components["schemas"]["AdminInvite"];
/** A new invite of the staff. */
export type NewStaffInvite = components["schemas"]["CreateInviteRequest"];

/** The kinds of a block on sign-ups, in the server's order. */
export const signupBlockKinds = [
  "email_domain",
  "email_hash",
  "ip_range",
] as const satisfies readonly SignupBlockKind[];
/** What a block does, the stronger first. */
export const signupBlockSeverities = ["reject", "require_approval"] as const satisfies readonly SignupBlockSeverity[];
/** The most characters of a block's comment and of a decision's note; the most characters of a value. */
export const signupBlockCommentLimit = 1000;
export const registrationNoteLimit = 1000;
export const signupBlockValueLimit = 320;
/** The most uses of an invite. */
export const inviteUsesLimit = 1_000_000;

/** A page of the registrations that wait for approval, the longest waiting first. */
function registrationsQuery({ queries }: AppData, cursor: string | undefined) {
  return queries.queryOptions("get", "/admin/registrations", {
    params: { query: cursor === undefined ? {} : { cursor } },
  });
}

/**
 * The page after `cursor` of the registrations that wait for approval, a registration decided on this screen staying
 * where it was with its decision until the screen reads the list again.
 */
export function useRegistrations(cursor: string | undefined): StaffPage<ModeratedRegistration> {
  return pageOf(useQuery({ ...registrationsQuery(useAppData(), cursor), placeholderData: keepPreviousData }));
}

/** Reads a page of the registrations, for a route's loader. */
export async function loadRegistrations(data: AppData, cursor: string | undefined): Promise<void> {
  await data.queryClient.query(registrationsQuery(data, cursor)).catch(() => undefined);
}

/**
 * Approves (`approve`) or rejects (`reject`) the registrations of `ids`, one after another, with `note` for the
 * moderators (none when empty); each decided puts its answer, without the email the list showed, in its place on the
 * page of `cursor`, and the counts and the accounts are read again. Resolves with what each answered, or the error.
 */
export async function decideRegistrations(
  data: AppData,
  cursor: string | undefined,
  ids: readonly string[],
  decision: "approve" | "reject",
  note: string,
): Promise<readonly { id: string; answer?: ModeratedRegistration; error?: unknown }[]> {
  const results: { id: string; answer?: ModeratedRegistration; error?: unknown }[] = [];
  for (const id of ids) {
    const init = { params: { path: { id } }, body: note === "" ? {} : { note } };
    try {
      const { data: answer } =
        decision === "approve"
          ? await data.api.POST("/admin/registrations/{id}/approve", init)
          : await data.api.POST("/admin/registrations/{id}/reject", init);
      if (answer === undefined) throw new Error("app-data: a decision answered without the registration");
      results.push({ id, answer });
    } catch (error) {
      results.push({ id, error });
    }
  }
  const decided = new Map(results.flatMap((r) => (r.answer === undefined ? [] : [[r.id, r.answer] as const])));
  type Page = { items: ModeratedRegistration[]; next_cursor?: string };
  await writeCached<Page>(data, registrationsQuery(data, cursor).queryKey, (page) => ({
    ...page,
    items: page.items.map((r) => decided.get(r.id) ?? r),
  }));
  staffChanged(data, ["/admin/accounts"]);
  return results;
}

/** Decides registrations, for a screen (`decideRegistrations`). */
export function useDecideRegistrations(
  cursor: string | undefined,
): (ids: readonly string[], decision: "approve" | "reject", note: string) => ReturnType<typeof decideRegistrations> {
  const data = useAppData();
  return (ids, decision, note) => decideRegistrations(data, cursor, ids, decision, note);
}

/** What the list of blocks shows: the blocks of one kind (every kind where left out), those in effect alone with `active`. */
export type SignupBlockQuery = {
  readonly kind?: SignupBlockKind | undefined;
  readonly active?: boolean | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the blocks on sign-ups, newest first. */
function signupBlocksQuery({ queries }: AppData, q: SignupBlockQuery) {
  return queries.queryOptions("get", "/admin/signup_blocks", {
    params: {
      query: {
        ...(q.kind === undefined ? {} : { kind: q.kind }),
        ...(q.active === true ? { active: true } : {}),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** The page of the blocks `q` asks for; the page shown before stays while the next is on its way. */
export function useSignupBlocks(q: SignupBlockQuery): StaffPage<SignupBlock> {
  return pageOf(useQuery({ ...signupBlocksQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** Reads a page of the blocks, for a route's loader. */
export async function loadSignupBlocks(data: AppData, q: SignupBlockQuery): Promise<void> {
  await data.queryClient.query(signupBlocksQuery(data, q)).catch(() => undefined);
}

/** One block on sign-ups. */
function signupBlockQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/signup_blocks/{id}", { params: { path: { id } } });
}

/** The block of `id`. */
export function useSignupBlock(id: string): StaffOne<SignupBlock> {
  return oneOf(useQuery(signupBlockQuery(useAppData(), id)));
}

/** Reads the block of `id`, for a route's loader. */
export async function loadSignupBlock(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(signupBlockQuery(data, id)).catch(() => undefined);
}

/** What a moderator does to the blocks on sign-ups; each rejects with the server's problem. */
export type SignupBlockWrites = {
  /** Adds a block, and resolves with it; a 422 names the member at fault (`value`, `comment`). */
  readonly add: (block: NewSignupBlock) => Promise<SignupBlock>;
  /** Adds a block for each value of a list, and resolves with what was added and skipped. */
  readonly importList: (list: SignupBlockImport) => Promise<SignupBlockImportResult>;
  /** Lifts the block of `id`, and resolves with it. */
  readonly lift: (id: string) => Promise<SignupBlock>;
};

/** What a moderator does to the blocks on sign-ups, for a screen. */
export function useSignupBlockWrites(): SignupBlockWrites {
  const data = useAppData();
  const changed = () => {
    staffChanged(data, ["/admin/signup_blocks"]);
  };
  return {
    add: async (block) => {
      const { data: answer } = await data.api.POST("/admin/signup_blocks", { body: block });
      if (answer === undefined) throw new Error("app-data: a block answered without itself");
      data.queryClient.setQueryData(signupBlockQuery(data, answer.id).queryKey, answer);
      changed();
      return answer;
    },
    importList: async (list) => {
      const { data: answer } = await data.api.POST("/admin/signup_blocks/import", { body: list });
      if (answer === undefined) throw new Error("app-data: an import answered without its result");
      changed();
      return answer;
    },
    lift: async (id) => {
      const { data: answer } = await data.api.POST("/admin/signup_blocks/{id}/lift", { params: { path: { id } } });
      if (answer === undefined) throw new Error("app-data: a lift answered without the block");
      await data.queryClient.cancelQueries({ queryKey: signupBlockQuery(data, id).queryKey });
      data.queryClient.setQueryData(signupBlockQuery(data, id).queryKey, answer);
      changed();
      return answer;
    },
  };
}

/** A page of the invites, newest first. */
function staffInvitesQuery({ queries }: AppData, cursor: string | undefined) {
  return queries.queryOptions("get", "/admin/invites", { params: { query: cursor === undefined ? {} : { cursor } } });
}

/** The page after `cursor` of the invites; the page shown before stays while the next is on its way. */
export function useStaffInvites(cursor: string | undefined): StaffPage<ModeratedInvite> {
  return pageOf(useQuery({ ...staffInvitesQuery(useAppData(), cursor), placeholderData: keepPreviousData }));
}

/** Reads a page of the invites, for a route's loader. */
export async function loadStaffInvites(data: AppData, cursor: string | undefined): Promise<void> {
  await data.queryClient.query(staffInvitesQuery(data, cursor)).catch(() => undefined);
}

/** What a holder of `manage_invites` does to invites; each rejects with the server's problem. */
export type StaffInviteWrites = {
  /** Creates an invite of the staff, and resolves with it; a 422 names `max_uses` or `expires_in`. */
  readonly create: (invite: NewStaffInvite) => Promise<ModeratedInvite>;
  /**
   * Revokes the invites of `ids`, one after another, each revoked put in its place on the page of `cursor`; resolves
   * with what each answered, or the error.
   */
  readonly revoke: (
    cursor: string | undefined,
    ids: readonly string[],
  ) => Promise<readonly { id: string; error?: unknown }[]>;
};

/** What a holder of `manage_invites` does to invites, for a screen. */
export function useStaffInviteWrites(): StaffInviteWrites {
  const data = useAppData();
  return {
    create: async (invite) => {
      const { data: answer } = await data.api.POST("/admin/invites", { body: invite });
      if (answer === undefined) throw new Error("app-data: an invite answered without itself");
      staffChanged(data, ["/admin/invites"]);
      return answer;
    },
    revoke: async (cursor, ids) => {
      const results: { id: string; answer?: ModeratedInvite; error?: unknown }[] = [];
      for (const id of ids) {
        try {
          const { data: answer } = await data.api.POST("/admin/invites/{id}/revoke", { params: { path: { id } } });
          results.push({ id, ...(answer === undefined ? {} : { answer }) });
        } catch (error) {
          results.push({ id, error });
        }
      }
      const revoked = new Map(results.flatMap((r) => (r.answer === undefined ? [] : [[r.id, r.answer] as const])));
      type Page = { items: ModeratedInvite[]; next_cursor?: string };
      await writeCached<Page>(data, staffInvitesQuery(data, cursor).queryKey, (page) => ({
        ...page,
        items: page.items.map((i) => revoked.get(i.id) ?? i),
      }));
      return results;
    },
  };
}
