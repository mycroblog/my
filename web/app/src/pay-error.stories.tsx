// SPDX-License-Identifier: AGPL-3.0-only

// The payment page "Error", a story per state of it (pay.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { payMeta, payStory } from "./pay.samples";

const meta = { title: "Pay/Error", ...payMeta } satisfies Meta;
export default meta;

export const Expired = payStory("error", "expired");
export const Unavailable = payStory("error", "unavailable");
