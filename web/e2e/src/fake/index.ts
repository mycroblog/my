// SPDX-License-Identifier: AGPL-3.0-only

// The fake API of the web client (ADR-0171 §5), for the component tests (on Bun) and the screens' stories (in the
// browser): the client API answering from the scenario or a seed, typed by the API specification, with what a test
// scripts on it; and the HTML the server renders a post's text as. The auth screens' page data is @my/e2e/fake/auth.

export {
  createFakeApi,
  fakeHost,
  fakeOrigin,
  fakeStatusPage,
  scenario,
  type Audience,
  type FakeApi,
  type FakeOptions,
  type FakePerson,
  type FakeSeed,
  type FakeStreamEvent,
} from "./api";
export { serverHtml } from "./html";
export type { Failure, Script, Scripted, Seen } from "./script";
export { UnimplementedError, type Route } from "./spec";
