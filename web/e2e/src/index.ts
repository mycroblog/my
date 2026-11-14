// SPDX-License-Identifier: AGPL-3.0-only

// @my/e2e (ADR-0015 §5): the scenario that the demo, the E2E tests and the screens' stories read, and its drawings. The
// fake API of the stories is @my/e2e/fake; the seed, the demo server and the specs run on Bun and Playwright alone.

export * from "./scenario";
export { drawingURL, drawSVG, type Drawing, type RGB, type Shape } from "./drawing";
