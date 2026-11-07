// SPDX-License-Identifier: AGPL-3.0-only
// The E2E projects that specs name (playwright.config.ts).

/**
 * The project every spec runs in, and the only one of a run without E2E_MATRIX=full: a spec that may run once per
 * run (a sign-up under the per-address rate limit, a check that depends on neither the viewport nor the locale) runs
 * in this one.
 */
export const primaryProject = "en-1440";
