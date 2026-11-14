// SPDX-License-Identifier: AGPL-3.0-only

// The instance's runtime configuration (ADR-0028 §1): the JSON data block <script type="application/json" id="config">
// of the HTML shell that the Go server writes (internal/interface/web, runtimeConfig). The build holds no value of
// any instance. Vite's stand-in shell (index.html) has none, and the app then takes the page's own origin and host.

/** What the app reads of the runtime configuration. */
export type RuntimeConfig = {
  /** The origin of the client API. */
  readonly apiOrigin: string;
  /** The instance's name in its default locale, or its host. */
  readonly instanceName: string;
  /** The instance's default locale. */
  readonly defaultLocale: string;
};

/** The configuration in `block` (the data block's text, or null without one), else the page's own at `location`. */
export function parseConfig(block: string | null, location: { origin: string; host: string }): RuntimeConfig {
  const fallback = { apiOrigin: location.origin, instanceName: location.host, defaultLocale: "en" };
  if (block === null || block.trim() === "") return fallback;
  const value: unknown = JSON.parse(block);
  if (typeof value !== "object" || value === null) throw new Error("app: the shell's config is not an object");
  const read = (key: keyof RuntimeConfig): string => {
    const v: unknown = (value as Record<string, unknown>)[key];
    return typeof v === "string" && v !== "" ? v : fallback[key];
  };
  return { apiOrigin: read("apiOrigin"), instanceName: read("instanceName"), defaultLocale: read("defaultLocale") };
}
