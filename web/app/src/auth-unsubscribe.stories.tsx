// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Unsubscribe", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Unsubscribe", ...authMeta } satisfies Meta;
export default meta;

export const Prompt = authStory("unsubscribe", "prompt");
export const Done = authStory("unsubscribe", "done");
