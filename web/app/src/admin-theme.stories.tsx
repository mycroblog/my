// SPDX-License-Identifier: AGPL-3.0-only

// The instance theme (FR-605, ADR-0059), on the fake server (app.samples.tsx) as its administrator (hana) reads it:
// the theme's screen with no draft and nothing published, with a draft not published yet, with it published after an
// earlier one (so that it can be restored), and with overrides an upgrade left without effect; the draft's form, from
// the defaults and from the draft; loading and failing. What follows a press (a pair under its minimum refused before
// sending, a value the server refused, published, the preview over the screens) is the route tests'; the draft's
// form with a pair under its minimum is a story, and Save's refusal of it a script's shot.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Administration/Theme",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin/theme", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the scenario's administrator reads it. */
const at = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "hana", ...more });

/** No draft saved and nothing published: the default theme, and Start a draft. */
export const Default: Story = { parameters: at("/admin/theme") };
/** A draft saved and not published: Publish, Preview and Edit the draft; the default theme in effect. */
export const Drafted: Story = { parameters: at("/admin/theme", { server: { theme: { draft: true } } }) };
/** The draft published after an earlier theme: the draft the same as the theme in effect, which can be restored. */
export const Published: Story = { parameters: at("/admin/theme", { server: { theme: { published: true } } }) };
/** The draft holds an override an upgrade left without effect, which the next save drops. */
export const Stale: Story = { parameters: at("/admin/theme", { server: { theme: { stale: true } } }) };
export const Loading: Story = { parameters: at("/admin/theme", { state: "loading" }) };
export const Failed: Story = { parameters: at("/admin/theme", { state: "error" }) };

/** The draft's form from the defaults: every color with its pairs' contrast in light and dark, and the other tokens. */
export const DraftDefault: Story = { parameters: at("/admin/theme/draft") };
/** The draft's form from the draft: a darker accent text in light, and rounder controls. */
export const Draft: Story = { parameters: at("/admin/theme/draft", { server: { theme: { draft: true } } }) };
/**
 * The draft's form with Accent text in light under its minimum on the page: its pair is marked, which Save refuses
 * before sending (shot by a script that presses Save).
 */
export const DraftBelow: Story = { parameters: at("/admin/theme/draft", { server: { theme: { low: true } } }) };
export const DraftLoading: Story = { parameters: at("/admin/theme/draft", { state: "loading" }) };
