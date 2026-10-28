// SPDX-License-Identifier: AGPL-3.0-only
// The runtime configuration of the instance (ADR-0028). The build is the same for every instance; the
// server writes the per-instance settings into the HTML shell as a JSON data block,
// <script type="application/json" id="config"> (runtimeConfig in internal/interface/web/shell.go).
// It is read and validated once at start-up and does not change while the page is open.
import { createContext, type ReactNode, use } from "react";
import { z } from "zod";

/** The id of the data block in the shell. */
const configElementId = "config";

/** A brand image of the instance (ADR-0116): the path of its SVG under /brand/ and the size of its viewBox. */
const brandImageSchema = z.object({
  url: z.string().regex(/^\/brand\//u),
  width: z.int().positive(),
  height: z.int().positive(),
});

const runtimeConfigSchema = z.object({
  /** The origin of the client API (ADR-0016). */
  apiOrigin: z.url({ protocol: /^https?$/ }),
  /** The instance's display name. */
  instanceName: z.string().min(1),
  /** The domain of local handles (@alice@{accountDomain}). */
  accountDomain: z.string().min(1),
  /** The instance's default locale, the same as <html lang> in the shell. */
  defaultLocale: z.string().min(1),
  /** The instance's own icon, logo, and dark logo; each is left out while the built-in image applies. */
  icon: brandImageSchema.optional(),
  logo: brandImageSchema.optional(),
  logoDark: brandImageSchema.optional(),
});

export type RuntimeConfig = z.infer<typeof runtimeConfigSchema>;

/** Why the configuration could not be read; the start-up error screen is the same for each. */
export class RuntimeConfigError extends Error {
  override name = "RuntimeConfigError";
}

/** Reads and validates the data block of `doc`. Throws a RuntimeConfigError when it is missing or invalid. */
export function readRuntimeConfig(doc: Document): RuntimeConfig {
  const element = doc.getElementById(configElementId);
  if (!(element instanceof HTMLScriptElement) || element.type !== "application/json") {
    throw new RuntimeConfigError(`the shell has no <script type="application/json" id="${configElementId}">`);
  }
  let json: unknown;
  try {
    json = JSON.parse(element.text);
  } catch (error) {
    throw new RuntimeConfigError("the runtime config is not JSON", { cause: error });
  }
  const result = runtimeConfigSchema.safeParse(json);
  if (!result.success) {
    throw new RuntimeConfigError(`the runtime config is invalid: ${z.prettifyError(result.error)}`, {
      cause: result.error,
    });
  }
  return result.data;
}

const ConfigContext = createContext<RuntimeConfig | null>(null);

export type ConfigProviderProps = { config: RuntimeConfig; children: ReactNode };

/** Gives `useConfig()` to its children. */
export function ConfigProvider({ config, children }: ConfigProviderProps) {
  return <ConfigContext value={config}>{children}</ConfigContext>;
}

/** Returns the runtime configuration. */
export function useConfig(): RuntimeConfig {
  const config = use(ConfigContext);
  if (config === null) throw new Error("useConfig needs a <ConfigProvider> above it");
  return config;
}
