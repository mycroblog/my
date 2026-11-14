// SPDX-License-Identifier: AGPL-3.0-only

// The auth screen "Sign up", a story per state of it (auth.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { authMeta, authStory } from "./auth.samples";

const meta = { title: "Auth/Sign up", ...authMeta } satisfies Meta;
export default meta;

export const Empty = authStory("sign-up", "empty");
export const FieldError = authStory("sign-up", "field-error");
export const Approval = authStory("sign-up", "approval");
export const External = authStory("sign-up", "external");
export const ProvidersOnly = authStory("sign-up", "providers-only");
export const Closed = authStory("sign-up", "closed");
