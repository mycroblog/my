// SPDX-License-Identifier: AGPL-3.0-only
// The part of the starlight-openapi API that src/api-reference.ts uses. The package ships its TypeScript sources, which
// do not pass the strict options of the workspace (tsconfig.base.json), so tsconfig.typecheck.json maps the import
// here and tsc never reads them; Astro and Vite still load the package itself. Keep this in step with the pinned
// version.
import type { StarlightPlugin, StarlightUserConfig } from "@astrojs/starlight/types";

type SidebarItemConfig = NonNullable<StarlightUserConfig["sidebar"]>[number];
type SidebarGroupConfig = Extract<SidebarItemConfig, { items: SidebarItemConfig[] }>;

export interface StarlightOpenAPISchemaConfig {
  /** The path under which the pages of the document are generated, e.g. `api`. */
  base: string;
  /** The path of the OpenAPI document, relative to the root of the Astro project, or its URL. */
  schema: string;
  sidebar?: {
    collapsed?: boolean;
    label?: string;
    /** The group made by createOpenAPISidebarGroup() that holds the generated pages in the sidebar. */
    group?: SidebarGroupConfig;
    operations?: {
      badges?: boolean;
      labels?: "operationId" | "path" | "summary";
      sort?: "alphabetical" | "document";
    };
    tags?: { sort?: "alphabetical" | "document" };
  };
}

export default function starlightOpenAPI(config: StarlightOpenAPISchemaConfig[]): StarlightPlugin;

export function createOpenAPISidebarGroup(): SidebarGroupConfig;
