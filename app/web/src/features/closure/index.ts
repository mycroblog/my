// SPDX-License-Identifier: AGPL-3.0-only
// The closure feature (FR-808, FR-809, ADR-0088): the notice kept at the top of every screen while the server's closure
// is announced, and the screen a signed-in user gets after the server has closed, where only the data export is left.
// Routes and other features use it through this file only (web-client/README.md A3).
export { ClosedScreen } from "./components/ClosedScreen";
export { ClosureBanner } from "./components/ClosureBanner";
