// SPDX-License-Identifier: AGPL-3.0-only

// The presentational components shared across the features of the web client (ADR-0025 §2, §4): built on
// @my/design-system, they take their data as props and fetch nothing. They write no Tailwind class of their own.

export { PostContent, type ContentEmoji, type PostContentProps } from "./post-content";
