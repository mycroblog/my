// SPDX-License-Identifier: AGPL-3.0-only
// The client API reference (ADR-0150 §5): starlight-openapi renders api/openapi.yaml, the bundle of api/spec/ that
// `scripts/task.sh api:generate` writes, so a change of the spec reaches the site with no other step. The plugin
// reads the file from the repository at build time; nothing of it is copied into app/docs. Its introduction is the
// document docs/developers/client-api.md: a link of a published document to api/openapi.yaml belongs on this
// reference (apiReferenceBase), not on the file in GitHub.
import type { StarlightPlugin, StarlightUserConfig } from "@astrojs/starlight/types";
import starlightOpenAPI, { createOpenAPISidebarGroup } from "starlight-openapi";

/** The path of the reference in the site, under the base of the version: the overview, then one page per operation. */
const apiReferenceBase = "api";

const referenceGroup = createOpenAPISidebarGroup();

const openAPI = starlightOpenAPI([
  {
    base: apiReferenceBase,
    // Relative to the root of the Astro project (app/docs).
    schema: "../../api/openapi.yaml",
    sidebar: {
      group: referenceGroup,
      label: "Client API reference",
      operations: { badges: true },
    },
  },
]);

export const apiReferencePlugin: StarlightPlugin = {
  name: "my-api-reference",
  hooks: {
    "config:setup": async (options) => {
      await openAPI.hooks["config:setup"]?.(options);
      // The plugin renders the descriptions of the spec with satteri, and its pages use Starlight's tabs, which do too,
      // while the pages are prerendered. Bundled into the prerender chunks, satteri cannot find its native binding from
      // dist/ under Bun's isolated installs, so it stays an external import, resolved from this package's dependency.
      options.addIntegration({
        name: "my-api-reference-externals",
        hooks: {
          "astro:config:setup": ({ updateConfig }) => {
            updateConfig({ vite: { ssr: { external: ["satteri"] } } });
          },
        },
      });
    },
  },
};

type SidebarItem = NonNullable<StarlightUserConfig["sidebar"]>[number];

/** The sidebar group of the reference: its overview and one group per tag of the spec, in the order of the spec. */
export const apiReferenceSidebar: SidebarItem = referenceGroup;
