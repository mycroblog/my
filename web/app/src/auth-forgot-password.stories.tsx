// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Forgot password", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Forgot password", ...authMeta } satisfies Meta;
export default meta;

export const Empty = authStory("forgot-password", "empty");
export const Locked = authStory("forgot-password", "locked");
export const Sent = authStory("forgot-password", "sent");
export const Unavailable = authStory("forgot-password", "unavailable");
