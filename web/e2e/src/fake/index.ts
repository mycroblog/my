// SPDX-License-Identifier: AGPL-3.0-only

// The fake API of the web client (ADR-0171 §5), for the component tests (on Bun) and the screens' stories (in the
// browser): the client API answering from the scenario or a seed, typed by the API specification, with what a test
// scripts on it; the HTML the server renders a post's text as; and the scenario's server and apps, for the stories of
// the screens the server's shell renders.

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
export { fakeExternalConfirmation, fakeTotpCode, type AccessOptions, type FakeLoginProviderKind } from "./access";
export { apps as scenarioApps, instance as scenarioInstance } from "../scenario";
export { serverHtml } from "./html";
export { memberNotificationRows, moderatorNotificationRows, type FakeNotificationRow } from "./settings";
export type { Failure, Script, Scripted, Seen } from "./script";
export { UnimplementedError, type Route } from "./spec";
export type { FakeVoice, FakeVoiceEvent } from "./voice";
export type { FakeEventPlan, FakeEvents } from "./events";
