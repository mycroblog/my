// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Signed in", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Signed in", ...authMeta } satisfies Meta;
export default meta;

export const Default = authStory("signed-in", "default");
