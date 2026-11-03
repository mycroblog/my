// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

// WebClientEpoch is the number of the last change that older web clients cannot work with
// (ADR-0141 §4). Raise it by hand, together with webClientEpoch in
// app/web/src/lib/webClientEpoch.ts, in a change that breaks open tabs of the web client
// without a deprecation period. A client with a smaller value offers the required update.
const WebClientEpoch = 1

// HeaderWebClientEpoch is the response header of the client API that carries WebClientEpoch.
const HeaderWebClientEpoch = "X-Web-Client-Epoch"
