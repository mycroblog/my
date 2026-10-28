// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the session feature (web-client/README.md §7): the signed-in user's own account.
import { http, HttpResponse } from "msw";
import { viewerFixture } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

export const handlers = [
  http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(viewerFixture)),
  ),
];
