// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Status", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Status", ...authMeta } satisfies Meta;
export default meta;

export const Pending = authStory("status", "pending");
export const Restricted = authStory("status", "restricted");
export const Deactivated = authStory("status", "deactivated");
