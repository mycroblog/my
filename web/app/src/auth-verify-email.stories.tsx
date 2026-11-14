// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Verify email", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Verify email", ...authMeta } satisfies Meta;
export default meta;

export const Prompt = authStory("verify-email", "prompt");
export const Done = authStory("verify-email", "done");
export const Invalid = authStory("verify-email", "invalid");
