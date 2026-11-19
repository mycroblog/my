// SPDX-License-Identifier: AGPL-3.0-only

// The payment page "Portal", a story per state of it (pay.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { payMeta, payStory } from "./pay.samples";

const meta = { title: "Pay/Portal", ...payMeta } satisfies Meta;
export default meta;

export const Payjp = payStory("portal", "payjp");
export const Changed = payStory("portal", "changed");
export const Paused = payStory("portal", "paused");
export const Empty = payStory("portal", "empty");
export const Fake = payStory("portal", "fake");
