// SPDX-License-Identifier: AGPL-3.0-only
// The client API reference (ADR-0150 §5): starlight-openapi renders api/openapi.yaml, the bundle of api/spec/ that
// `scripts/task.sh api:generate` writes, so a change of the spec reaches the site with no other step. The plugin
// reads the copy of it that the copy step writes before each build (scripts/import-docs.ts), in which the schemas that
// other schemas hold link to their own pages instead of being expanded on every operation (src/api-schemas.ts); the
// schema pages are src/pages/api/schemas/. Its introduction is the document docs/developers/client-api.md: a link of a
// published document to api/openapi.yaml belongs on this reference (apiReferenceBase), not on the file in GitHub.
// In the sidebar, the group of the reference lists its overview and one link per tag of the spec, to the tag's page;
// only the tag of the current page lists its operations (src/api-sidebar.ts, a route middleware that runs after the
// plugin's), so a page's sidebar does not grow with every operation of the API, on every page and in every locale.
import { createRequire } from "node:module";
import { dirname } from "node:path";
import type { StarlightPlugin, StarlightUserConfig } from "@astrojs/starlight/types";
import starlightOpenAPI, { createOpenAPISidebarGroup } from "starlight-openapi";
import { apiReferenceBase, referenceSpecPath } from "./api-schemas";
import { apiReferenceLabel } from "./api-sidebar";

/**
 * The sources of the pinned starlight-openapi, whose component and example generator the schema pages render a schema
 * with, so that it looks as on the operation pages. The package exports neither, so the pages import them by this
 * alias of its directory (`starlight-openapi-sources/components/...`).
 */
const openAPISources = dirname(createRequire(import.meta.url).resolve("starlight-openapi/package.json"));

const referenceGroup = createOpenAPISidebarGroup();

const openAPI = starlightOpenAPI([
  {
    base: apiReferenceBase,
    // Relative to the root of the Astro project (app/docs).
    schema: referenceSpecPath,
    sidebar: {
      group: referenceGroup,
      label: apiReferenceLabel,
      operations: { badges: true },
    },
  },
]);

export const apiReferencePlugin: StarlightPlugin = {
  name: "my-api-reference",
  hooks: {
    "config:setup": async (options) => {
      await openAPI.hooks["config:setup"]?.(options);
      // After the plugin's middleware, which fills the group of the reference, both in the "post" bucket: Starlight
      // runs the middlewares of one bucket in the order they are added.
      options.addRouteMiddleware({ entrypoint: "./src/api-sidebar.ts", order: "post" });
      // The plugin renders the descriptions of the spec with satteri, and its pages use Starlight's tabs, which do too,
      // while the pages are prerendered. Bundled into the prerender chunks, satteri cannot find its native binding from
      // dist/ under Bun's isolated installs, so it stays an external import, resolved from this package's dependency.
      // The alias lets the schema pages import the plugin's components (openAPISources).
      options.addIntegration({
        name: "my-api-reference-vite",
        hooks: {
          "astro:config:setup": ({ updateConfig }) => {
            updateConfig({
              vite: {
                ssr: { external: ["satteri"] },
                resolve: { alias: { "starlight-openapi-sources": openAPISources } },
              },
            });
          },
        },
      });
    },
  },
};

type SidebarItem = NonNullable<StarlightUserConfig["sidebar"]>[number];

/**
 * The sidebar group of the reference: its overview and one entry per tag of the spec, in the order of the spec, a link
 * to the tag's page or, for the tag of the current page, a group of its operations (src/api-sidebar.ts).
 */
export const apiReferenceSidebar: SidebarItem = referenceGroup;
