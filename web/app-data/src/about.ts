// SPDX-License-Identifier: AGPL-3.0-only
// What the about page and the server's documents show besides the instance (FR-705, FR-718; ADR-0072 §13): a document
// the instance published (its terms, its privacy policy, its commerce disclosure), and the domains it moderates, which
// it lists only to whom `domain_actions_visibility` lets read them.
import type { components } from "@mycroblog/mykit";
import type { Instance } from "./instance";

type Schemas = components["schemas"];

/** A document of the instance: its kind, its HTML as the server sanitized it, and when it was last revised. */
export type InstanceDocument = Schemas["InstanceDocument"];
/** Which document: the terms of service, the privacy policy, or the commerce disclosure of the supporter plans. */
export type InstanceDocumentKind = Schemas["InstanceDocumentKind"];
/** A domain the instance moderates, as it lists it publicly: limited or suspended, with its public comment. */
export type InstanceDomainAction = Schemas["InstanceDomainAction"];

/**
 * Whether `instance` lists its moderated domains to the reader: to everyone, or, with `signed_in`, to a reader with a
 * session (`signedIn`).
 */
export function domainActionsShown(
  instance: Pick<Instance, "domain_actions_visibility"> | undefined,
  signedIn: boolean,
): boolean {
  const visibility = instance?.domain_actions_visibility;
  return visibility === "everyone" || (visibility === "signed_in" && signedIn);
}
