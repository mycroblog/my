// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Reset password", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Reset password", ...authMeta } satisfies Meta;
export default meta;

export const Empty = authStory("reset-password", "empty");
export const FieldError = authStory("reset-password", "field-error");
export const Done = authStory("reset-password", "done");
export const Invalid = authStory("reset-password", "invalid");
