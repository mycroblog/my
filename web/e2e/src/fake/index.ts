// SPDX-License-Identifier: AGPL-3.0-only

// The fake of the server for the screens' stories (in the browser): the client API answering from the scenario, the
// auth screens' page data, and the HTML the server renders a post's text as.

export {
  createFakeApi,
  fakeHost,
  fakeOrigin,
  fakeStatusPage,
  type Audience,
  type FakeApi,
  type FakeOptions,
} from "./api";
export { authScreens, type AuthScreenStates } from "./auth";
export { serverHtml } from "./html";
