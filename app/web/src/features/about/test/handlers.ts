// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the about feature (web-client/README.md §7): the instance's public list of moderated domains.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase } from "../../../test/msw/http";

/** A suspended domain with a public comment, and a limited one that the moderators obfuscated. */
export const publicDomainActionsFixture: Schemas["InstanceDomainAction"][] = [
  {
    domain: "spam.example",
    digest: "1f6c8a7bfe4b3c0d2e9a8b7c6d5e4f30112233445566778899aabbccddeeff00",
    kind: "suspend",
    comment: "Spam and harassment",
  },
  { domain: "bad.*****le", digest: "2a7d9b8c0f5c4d1e3f0b9c8d7e6f504122334455667788990011aabbccddeeff", kind: "limit" },
];

export const handlers = [
  http.get(`${apiBase}/instance/domain_actions`, () => HttpResponse.json({ items: publicDomainActionsFixture })),
];
