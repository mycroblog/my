// SPDX-License-Identifier: AGPL-3.0-only

// The payment page "Checkout", a story per state of it (pay.samples.tsx).

import type { Meta } from "@storybook/react-vite";
import { payMeta, payStory } from "./pay.samples";

const meta = { title: "Pay/Checkout", ...payMeta } satisfies Meta;
export default meta;

export const Payjp = payStory("checkout", "payjp");
export const TestMode = payStory("checkout", "test-mode");
export const Refused = payStory("checkout", "refused");
export const Fake = payStory("checkout", "fake");
export const FakeFailed = payStory("checkout", "fake-failed");
export const Tip = payStory("checkout", "tip");
