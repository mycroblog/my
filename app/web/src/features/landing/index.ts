// SPDX-License-Identifier: AGPL-3.0-only
// The landing feature: the page that people who are not signed in see at the root address (`/`): the server's calling
// card and the three sections under it (what kind of place it is, what people agree to, how to join), with the ways to
// sign in, to sign up and to follow from another server (docs/architecture/web-client/screens/landing.md). Routes and other features use it through this file only (web-client/README.md A3).
export { instanceQueryOptions } from "./api/landing";
export { Landing } from "./components/Landing";
export { useSignUpHref } from "./hooks/useLanding";
