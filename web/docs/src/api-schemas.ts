// SPDX-License-Identifier: AGPL-3.0-only
// The schemas of the client API reference (src/api-reference.ts). starlight-openapi dereferences the whole document
// and renders the schema of every request and response fully expanded, so an operation page would repeat the tree of
// every schema it reaches (a page of interaction requests would hold the whole Status and Account, each with the
// schemas they hold, and so on every page that uses them). The site gives the plugin a copy of api/openapi.yaml
// instead, which scripts/import-docs.ts writes (referenceSpec): in it, an object schema of components/schemas that a
// field of another schema holds is a link to the page of that schema (src/pages/api/schemas/). An operation page
// shows the fields of the schemas it names and links the ones they hold, and each schema page shows its own fields
// the same way. Nothing is dropped: every object schema has its page, reached from every field that holds it.
// Some references stay inline, as they say little or are the point of the schema that holds them: a scalar schema
// (Id, an enum), the members of a multi-member allOf (the schema that composes them has their fields), and the items
// of an array (a page of statuses shows the fields of a status). The examples the plugin generates follow the same
// cut: a linked schema is `{}` in the example of the schema that holds it, and its own page has its example.
import { readFileSync } from "node:fs";
import { slug } from "github-slugger";
import { base } from "./published";

/** The path of the reference within the site, under the base of the version (as starlight-openapi's `base`). */
export const apiReferenceBase = "api";

/** The path of the schema pages within the site, under the base of the version. */
const apiSchemasBase = `${apiReferenceBase}/schemas`;

/** The copy of api/openapi.yaml that starlight-openapi renders, relative to the root of the Astro project. */
export const referenceSpecPath = "generated/openapi.json";

/**
 * The sidebar of the schema pages, relative to the root of the Astro project: the site's, with the group of the
 * reference cut to its overview and the list of schemas. The full group lists every operation, which would make most
 * of each of the many schema pages; they are reached from the operations and the list. scripts/import-docs.ts writes
 * it, since the sidebar (src/sidebar.ts) reads the repository, out of reach of the prerendered pages.
 */
export const schemaSidebarPath = "generated/schema-sidebar.json";

/** The group of the reference in the sidebar of the schema pages. */
export const schemaSidebarGroup = {
  label: "Client API reference",
  items: [
    { label: "Overview", link: `/${apiReferenceBase}/` },
    { label: "Schemas", link: `/${apiSchemasBase}/` },
  ],
};

/** The sidebar of the schema pages that scripts/import-docs.ts wrote. */
export function schemaSidebar(): unknown[] {
  return JSON.parse(readFileSync(schemaSidebarPath, "utf8")) as unknown[];
}

type Json = null | boolean | number | string | Json[] | { [key: string]: Json };
type JsonObject = { [key: string]: Json };

/** A schema object, with the `$ref`s of the spec resolved (dereference) or not. */
type SchemaObject = { [key: string]: unknown };

const schemaRefPrefix = "#/components/schemas/";

function isObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** The name of the component schema `node` is a reference to, if it is one. */
function refName(node: unknown): string | undefined {
  if (!isObject(node)) {
    return undefined;
  }
  const ref = node["$ref"];
  return typeof ref === "string" && ref.startsWith(schemaRefPrefix) ? ref.slice(schemaRefPrefix.length) : undefined;
}

function componentSchemas(spec: JsonObject): JsonObject {
  const components = spec["components"];
  const schemas = isObject(components) ? components["schemas"] : undefined;
  return isObject(schemas) ? schemas : {};
}

/** Whether the schema has fields (an object, or a composition of schemas), which earns it a page of its own. */
function hasPage(schema: Json | undefined): boolean {
  if (!isObject(schema)) {
    return false;
  }
  const type = schema["type"];
  return (
    type === "object" ||
    (Array.isArray(type) && type.includes("object")) ||
    ["properties", "additionalProperties", "allOf", "oneOf", "anyOf"].some((key) => key in schema)
  );
}

/** The page slug of the schema `name` under apiSchemasBase. */
function schemaSlug(name: string): string {
  return slug(name);
}

/** The site URL of the page of the schema `name`. */
export function schemaURL(name: string): string {
  return `${base}${apiSchemasBase}/${schemaSlug(name)}/`;
}

/**
 * The field that stands for the schema `name` where another schema holds it: the type of the schema and a link to its
 * page, after the description of the field (`siblings`, the keywords written next to the reference).
 */
function link(name: string, target: JsonObject, siblings: JsonObject): JsonObject {
  const type = target["type"];
  const description = typeof siblings["description"] === "string" ? siblings["description"].trimEnd() : "";
  const see = `Schema: [\`${name}\`](${schemaURL(name)})`;
  return {
    ...siblings,
    type: typeof type === "string" || Array.isArray(type) ? type : "object",
    description: description === "" ? see : `${description}\n\n${see}`,
  };
}

/** The schema `node` of the component schemas with every schema it holds that has a page replaced by its link. */
function rewrite(node: Json, schemas: JsonObject): Json {
  if (Array.isArray(node)) {
    return node.map((item) => rewrite(item, schemas));
  }
  if (!isObject(node)) {
    return node;
  }
  const name = refName(node);
  if (name !== undefined) {
    const { $ref: _, ...siblings } = node;
    return hasPage(schemas[name]) ? link(name, schemas[name] as JsonObject, siblings) : node;
  }
  // The idiom of a field that describes the schema it holds: `description` next to `allOf: [{$ref}]`.
  const allOf = node["allOf"];
  if (Array.isArray(allOf) && allOf.length === 1) {
    const only = refName(allOf[0]);
    if (only !== undefined && hasPage(schemas[only])) {
      const { allOf: _, ...siblings } = node;
      return link(only, schemas[only] as JsonObject, rewriteKeywords(siblings, schemas));
    }
  }
  return rewriteKeywords(node, schemas);
}

/**
 * The keywords of the schema `node`, each subschema rewritten; the members of a composition (allOf) and the items of
 * an array stay inline.
 */
function rewriteKeywords(node: JsonObject, schemas: JsonObject): JsonObject {
  const out: JsonObject = {};
  for (const [key, value] of Object.entries(node)) {
    if (key === "allOf" && Array.isArray(value) && value.length > 1) {
      out[key] = value.map((member) => (refName(member) !== undefined ? member : rewrite(member, schemas)));
    } else if (key === "items" && refName(value) !== undefined) {
      out[key] = value;
    } else if (key === "properties" && isObject(value)) {
      out[key] = Object.fromEntries(Object.entries(value).map(([field, schema]) => [field, rewrite(schema, schemas)]));
    } else if (key === "example" || key === "examples" || key === "enum" || key === "const" || key === "default") {
      out[key] = value;
    } else {
      out[key] = rewrite(value, schemas);
    }
  }
  return out;
}

/**
 * A resolver of the `$ref`s to component schemas in the spec: `resolve(node)` is the node with every reference in it
 * replaced by the schema it names, each schema resolved once and shared, so that a recursive schema is a cycle (as
 * starlight-openapi's dereference makes it) and not an endless tree.
 */
function resolver(spec: JsonObject): (node: unknown) => unknown {
  const schemas = componentSchemas(spec);
  const resolved = new Map<string, SchemaObject>();
  function resolve(node: unknown): unknown {
    if (Array.isArray(node)) {
      return node.map(resolve);
    }
    if (!isObject(node)) {
      return node;
    }
    const name = refName(node);
    if (name !== undefined) {
      const { $ref: _, ...siblings } = node;
      const target = schema(name);
      return Object.keys(siblings).length === 0 ? target : { ...target, ...(resolve(siblings) as SchemaObject) };
    }
    return Object.fromEntries(Object.entries(node).map(([key, value]) => [key, resolve(value)]));
  }
  function schema(name: string): SchemaObject {
    const done = resolved.get(name);
    if (done !== undefined) {
      return done;
    }
    const out: SchemaObject = {};
    resolved.set(name, out);
    const source = schemas[name];
    if (!isObject(source)) {
      throw new Error(`api/openapi.yaml: the schema ${name} is referenced but not defined`);
    }
    Object.assign(out, resolve(source));
    return out;
  }
  return resolve;
}

/** The copy of the spec that starlight-openapi renders: the spec with the schemas that schemas hold linked. */
export function referenceSpec(spec: JsonObject): JsonObject {
  const schemas = componentSchemas(spec);
  const slugs = new Map<string, string>();
  for (const name of Object.keys(schemas)) {
    const other = slugs.get(schemaSlug(name));
    if (other !== undefined) {
      throw new Error(`api/openapi.yaml: the schemas ${other} and ${name} would have the same page`);
    }
    slugs.set(schemaSlug(name), name);
  }
  const out = structuredClone(spec);
  (out["components"] as JsonObject)["schemas"] = Object.fromEntries(
    Object.entries(schemas).map(([name, schema]) => [name, rewriteKeywords(schema as JsonObject, schemas)]),
  );
  const info = out["info"];
  if (isObject(info)) {
    const intro = typeof info["description"] === "string" ? info["description"].trimEnd() : "";
    info["description"] =
      `${intro}\n\nEach object schema of the requests and responses has a page of its own, listed under ` +
      `[Schemas](${base}${apiSchemasBase}/).\n`;
  }
  return out;
}

/** A schema that has a page: its name, its slug, and its schema resolved as the copy has it. */
export interface SchemaPage {
  name: string;
  slug: string;
  schema: SchemaObject;
}

/**
 * The schemas that have a page, in the order of the spec, from the copy that scripts/import-docs.ts writes. Its path
 * is relative to the root of the Astro project (web/docs), the working directory of the build, like
 * src/page-markdown.ts: the schema pages read it while they are prerendered.
 */
export function schemaPages(): SchemaPage[] {
  const spec = JSON.parse(readFileSync(referenceSpecPath, "utf8")) as JsonObject;
  const schemas = componentSchemas(spec);
  const resolve = resolver(spec);
  return Object.keys(schemas)
    .filter((name) => hasPage(schemas[name]))
    .map((name) => ({
      name,
      slug: schemaSlug(name),
      schema: resolve({ $ref: `${schemaRefPrefix}${name}` }) as SchemaObject,
    }));
}

/** The first sentence of the description of the schema, if it has one, as plain text. */
export function schemaSummary(page: SchemaPage): string | undefined {
  const description = page.schema["description"];
  if (typeof description !== "string") {
    return undefined;
  }
  const text = description.replace(/\s+/g, " ").replace(/`/g, "").trim();
  const sentence = /^.*?[.:](?=\s|$)/.exec(text)?.[0] ?? text;
  return sentence === "" ? undefined : sentence;
}

/** The meta description of the page of the schema. */
export function schemaDescription(page: SchemaPage): string {
  return schemaSummary(page) ?? `The ${page.name} schema of the client API.`;
}
