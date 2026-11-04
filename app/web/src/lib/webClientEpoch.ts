// SPDX-License-Identifier: AGPL-3.0-only
// The number of the last change that older clients cannot work with (ADR-0028 §12). Raise it by hand, together with the
// server's clientapi.WebClientEpoch, in a change that breaks open tabs of the web client without a deprecation period
// (docs/agents/workflow.md). It is not a version: a build does not change it, so it leaves the assets' hashes alone.
export const webClientEpoch = 1;

/** The response header of the client API that carries the server's value. */
export const webClientEpochHeader = "X-Web-Client-Epoch";
