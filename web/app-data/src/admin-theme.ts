// SPDX-License-Identifier: AGPL-3.0-only
// The instance theme as a holder of `manage_settings` edits it (FR-605, ADR-0059): the draft token set and the set in
// effect, each with the overrides that no longer apply since an upgrade; saving the draft whole, publishing it, and
// restoring the set published before the one in effect, each refused with every reason (`themeErrorsOf`) when a set
// breaks a rule. And the preview of the draft: while the reader previews it, the draft's stylesheet, which the app
// lays over every screen of this tab (`useThemePreview`), read again at each save. A publication changes what every
// visitor loads (`Instance.theme`), so it reads the instance again.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { instanceQuery } from "./instance";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";

/** The instance theme as its administrators edit it: the draft, and the publication in effect. */
export type AdminTheme = components["schemas"]["AdminTheme"];
/** A saved token set of the theme. */
export type ThemeSet = components["schemas"]["ThemeSet"];
/** The override of one design token: `light` and `dark` for a color, `value` for another kind. */
export type ThemeTokenValue = components["schemas"]["ThemeTokenValue"];
/** A stored override that no longer applies, and why. */
export type StaleThemeToken = components["schemas"]["StaleThemeToken"];
/** One reason a token set cannot be saved or published. */
export type ThemeError = components["schemas"]["ThemeError"];

/** The theme, as its administrators edit it. */
export function adminThemeQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/theme");
}

/** Reads the theme, for a route's loader. */
export async function loadAdminTheme(data: AppData): Promise<void> {
  await data.queryClient.query({ ...adminThemeQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The theme and where its read is. */
export function useAdminTheme(): ScreenRead & { readonly theme: AdminTheme | undefined } {
  const query = useQuery(adminThemeQuery(useAppData()));
  return { ...readOf([query]), theme: query.data };
}

/** Every reason the server refused a token set for (`theme_errors` of `/api/v1/problems/invalid-theme`); else none. */
export function themeErrorsOf(error: unknown): readonly ThemeError[] | undefined {
  if (!(error instanceof ProblemError) || error.type !== "/api/v1/problems/invalid-theme") return undefined;
  const errors = error.problem["theme_errors"];
  return Array.isArray(errors) ? (errors as ThemeError[]) : undefined;
}

/** Whether `error` is the server's 404: no draft was saved (publish), or no other set was ever published (restore). */
export function themeNothingThere(error: unknown): boolean {
  return error instanceof ProblemError && error.status === 404;
}

/** The writes of the theme; each resolves with the theme as the server answered it, or rejects with its problem. */
export type AdminThemeWrites = {
  /** Saves `tokens` as the draft, which replaces the draft before as a whole; none resets the theme once published. */
  readonly saveDraft: (tokens: readonly ThemeTokenValue[]) => Promise<AdminTheme>;
  /** Publishes the draft under a new revision, which every visitor loads. */
  readonly publish: () => Promise<AdminTheme>;
  /** Publishes again the set published before the one in effect; the draft stays as it is. */
  readonly restore: () => Promise<AdminTheme>;
};

/** The writes of the theme, for a screen. */
export function useAdminThemeWrites(): AdminThemeWrites {
  const data = useAppData();
  const written = async (theme: AdminTheme | undefined, published: boolean): Promise<AdminTheme> => {
    if (theme === undefined) throw new Error("app-data: a write of the theme answered without the theme");
    const { queryClient } = data;
    const key = adminThemeQuery(data).queryKey;
    await queryClient.cancelQueries({ queryKey: key });
    queryClient.setQueryData(key, theme);
    // The draft's stylesheet is keyed by the draft it is of, which a save replaces.
    void queryClient.invalidateQueries({ queryKey: draftStylesheetKey });
    if (published) void queryClient.invalidateQueries({ queryKey: instanceQuery(data).queryKey });
    return theme;
  };
  return {
    saveDraft: async (tokens) =>
      written((await data.api.PUT("/admin/theme/draft", { body: { tokens: [...tokens] } })).data, false),
    publish: async () => written((await data.api.POST("/admin/theme/publish")).data, true),
    restore: async () => written((await data.api.POST("/admin/theme/restore")).data, true),
  };
}

/** The key of what the cache keeps of the draft's stylesheet, by the draft's ID. */
const draftStylesheetKey = ["admin-theme-draft-stylesheet"] as const;

/**
 * Whether the reader previews the draft, kept for the session's data (one a tab): a value of its own, not a query of
 * the cache, which a session's end clears and which a query with initial data then filled again under every observer.
 */
type PreviewSwitch = { on: boolean; readonly heard: Set<() => void> };
const previews = new WeakMap<AppData, PreviewSwitch>();

/** The switch of the preview of `data`'s session. */
function previewOf(data: AppData): PreviewSwitch {
  let found = previews.get(data);
  if (found === undefined) {
    found = { on: false, heard: new Set() };
    previews.set(data, found);
  }
  return found;
}

/** Turns the preview of `data`'s session on or off, and tells every screen that shows it. */
function setPreview(data: AppData, on: boolean): void {
  const preview = previewOf(data);
  if (preview.on === on) return;
  preview.on = on;
  for (const hear of preview.heard) hear();
}

/** The preview of the draft: whether it is on, the stylesheet to lay over the page while it is, and the switches. */
export type ThemePreview = {
  readonly on: boolean;
  /** The draft's stylesheet, while the preview is on and once it is read. */
  readonly stylesheet: string | undefined;
  readonly start: () => void;
  readonly stop: () => void;
};

/**
 * The preview of the draft in this tab: off until a screen starts it, then the stylesheet of the draft as the server
 * would serve it once published (`GET /admin/theme/draft/stylesheet`), read again at each save of the draft. Nothing is
 * asked while it is off.
 */
export function useThemePreview(): ThemePreview {
  const data = useAppData();
  const preview = previewOf(data);
  const on = useSyncExternalStore(
    (hear) => {
      preview.heard.add(hear);
      return () => {
        preview.heard.delete(hear);
      };
    },
    () => preview.on,
    () => false,
  );
  const draft = useQuery({ ...adminThemeQuery(data), enabled: on }).data?.draft;
  const stylesheet = useQuery({
    queryKey: [...draftStylesheetKey, draft?.id ?? ""],
    queryFn: async () => {
      const { data: text } = await data.api.GET("/admin/theme/draft/stylesheet", { parseAs: "text" });
      return typeof text === "string" ? text : "";
    },
    enabled: on && draft !== undefined,
    staleTime: Infinity,
  }).data;
  return {
    on,
    stylesheet: on ? stylesheet : undefined,
    start: () => {
      setPreview(data, true);
    },
    stop: () => {
      setPreview(data, false);
    },
  };
}
