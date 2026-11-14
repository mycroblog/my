// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Two-factor code", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Two-factor code", ...authMeta } satisfies Meta;
export default meta;

export const Empty = authStory("two-factor", "empty");
export const WrongCode = authStory("two-factor", "wrong-code");
