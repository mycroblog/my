// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Sign in", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Sign in", ...authMeta } satisfies Meta;
export default meta;

export const Empty = authStory("sign-in", "empty");
export const Filled = authStory("sign-in", "filled");
export const ServerError = authStory("sign-in", "server-error");
export const Locked = authStory("sign-in", "locked");
export const PasskeyError = authStory("sign-in", "passkey-error");
export const HumanCheck = authStory("sign-in", "human-check");
export const ProvidersOnly = authStory("sign-in", "providers-only");
export const OwnLogo = authStory("sign-in", "own-logo");
