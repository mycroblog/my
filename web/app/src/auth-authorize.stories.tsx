// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Authorize", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Authorize", ...authMeta } satisfies Meta;
export default meta;

export const Consent = authStory("authorize", "consent");
export const Native = authStory("authorize", "native");
export const OwnLogo = authStory("authorize", "own-logo");
