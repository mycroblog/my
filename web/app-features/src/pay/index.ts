// SPDX-License-Identifier: AGPL-3.0-only

// The payment pages (ADR-0074 §11), a subpath of their own (@my/app-features/pay), so that the payment entry's module
// graph holds this feature and what it imports and nothing of the other features: every script the page may run is in
// the build's inventory with its integrity, and the build refuses a graph that imports a chunk on demand
// (web/app/scripts/payment-scripts.ts).

export { parsePayPage, type PayPage, type PayScreenName } from "./data";
export { payjpScript, type Payjp, type PayjpCardElement, type PayjpFactory } from "./payjp";
export { PayScreen } from "./screen";
