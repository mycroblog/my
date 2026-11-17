// SPDX-License-Identifier: AGPL-3.0-only

// The layout parts of @my/design-system (ADR-0171 §3), by file: the components whose only job is to place what they
// are given. restrict/layout-parts (styles.js) lets these alone write layout, and restrict/viewport-breakpoints lets
// the page frames among them alone decide by the viewport (ADR-0025 §4).
//
// A component is a layout part when all of this holds:
// - it renders only elements that wrap what it is given (its children, and the parts it takes as props), with the
//   attributes that name or describe them: no text, glyph, picture or control of its own;
// - it does nothing but place: no state, and no effect or handler other than one that measures and places;
// - its classes place and space, with the resets of the element it uses (`list-none`, `m-0`, `p-0`). The frame of a
//   page also sets the page's base (its surface and its font) and the surface of a bar it holds in place.
//
// So a section under a heading that sets the heading's type, a surface that stacks what is on it, an overlay with its
// title and its dismissal, and a feed with its scrolling are not layout parts: what places inside them becomes one
// when it is taken out of them. A name here that its file no longer declares fails the lint of the file.

/** @type {Record<string, string[]>} */
export const LAYOUT_PARTS = {
  "web/design-system/src/shell.tsx": [
    "Frame",
    "FrameSide",
    "FrameDock",
    "FrameMain",
    "FrameAside",
    "FrameBanners",
    "FrameActions",
    "NavList",
    "SectionList",
  ],
  "web/design-system/src/form-page.tsx": ["FormPage", "Form", "FormActions", "FormChoice", "FormLinks", "FormList"],
  "web/design-system/src/list.tsx": ["ScreenStack", "ScreenHead", "RowList", "RowItem", "ButtonPair"],
  "web/design-system/src/entry.tsx": ["Stack", "ActionRow"],
  "web/design-system/src/chip.tsx": ["ChipRow"],
  "web/design-system/src/thread.tsx": ["ThreadReply"],
  "web/design-system/src/drafts.tsx": ["DraftSections", "DraftList"],
  "web/design-system/src/history.tsx": ["VersionList"],
  "web/design-system/src/composer.tsx": ["AttachmentList", "PollSettings"],
  "web/design-system/src/article-editor.tsx": ["ArticlePane"],
  // The fixtures that prove the rules (restrictions.test.js).
  "web/design-system/fixtures/layout.tsx": ["Row", "Shelf", "Gone"],
  "web/design-system/fixtures/viewport.tsx": ["Page", "Column"],
};

/**
 * The frames of a page: the app's (shell.tsx, ADR-0110 §10) and that of a page of its own outside it (form-page.tsx).
 * @type {Record<string, string[]>}
 */
export const PAGE_FRAMES = {
  "web/design-system/src/shell.tsx": ["Frame", "FrameSide", "FrameDock", "FrameMain", "FrameAside"],
  "web/design-system/src/form-page.tsx": ["FormPage"],
  "web/design-system/fixtures/viewport.tsx": ["Page"],
};
