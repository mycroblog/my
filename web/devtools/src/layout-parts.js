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
//   page also sets the page's base (its surface and its font) and the surface of a bar it holds in place, and the
//   frame of a modal the backdrop it lays over the page behind it. A part of a frame that the viewport rearranges (a
//   place of the navigation, the call's bar) also holds what of its look changes with its place.
//
// There are two kinds. The generic ones (layout.tsx) know nothing of mycroblog: their props name an arrangement or a
// token of the theme's spacing, never a class, and every other part takes its layout from them, rendered as the
// element it would have written (their `as`), so that no element is added. A part named in its own file has an
// arrangement that only it has, which the generic ones' props do not say: the frame of a page, the frame of a modal
// over it, a place that depends on what holds the part. A part that only names a use of the generic ones (a stack of
// a screen's parts, a row of chips) writes no layout and is not listed here.
//
// So a section under a heading that sets the heading's type, a surface that stacks what is on it, and a feed with its
// scrolling are not layout parts: they compose them. Nor is an overlay: where it stands over the viewport is the
// modal's frame's to say (modal.tsx, by the place the overlay names) or React Aria's popover's, by its trigger; how
// what it holds is arranged is the generic parts'; its surface, its title and its dismissal are its own. Nor is a
// control: it is laid out as a generic part (a button is a `Row` rendered as its element). A name here that its file
// no longer declares fails the lint of the file.

/** @type {Record<string, string[]>} */
export const LAYOUT_PARTS = {
  // The generic parts, and `part`, the function that renders each of them.
  "web/design-system/src/layout.tsx": ["Box", "Row", "Column", "Stack", "Grid", "Layers", "Pin", "Sticky", "part"],
  "web/design-system/src/shell.tsx": [
    "Frame",
    "FrameSide",
    "FrameDock",
    "FrameMain",
    "FrameAside",
    "MarkPlace",
    "EntryLabel",
    "NavEntry",
    "FoldRow",
    "CallPart",
  ],
  "web/design-system/src/form-page.tsx": ["FormPage", "GlyphLine"],
  "web/design-system/src/list.tsx": ["GroupPlace", "ResultPlace"],
  "web/design-system/src/modal.tsx": ["ModalFrame", "ModalPanel"],
  "web/design-system/src/dialog.tsx": ["PanelFrame"],
  "web/design-system/src/article.tsx": ["ArticleFlow"],
  "web/design-system/src/card.tsx": ["AuthorLine"],
  "web/design-system/src/content.tsx": ["ContentBody"],
  "web/design-system/src/feed.tsx": ["NewItemsPlace"],
  "web/design-system/src/thread.tsx": ["ThreadReply"],
  "web/design-system/src/drafts.tsx": ["DraftSections", "DraftList"],
  "web/design-system/src/history.tsx": ["VersionList"],
  "web/design-system/src/composer.tsx": [
    "AttachmentList",
    "AttachmentLine",
    "PollSettings",
    "ComposerPanel",
    "ToolWords",
    "NoticePlace",
  ],
  "web/design-system/src/suggestions.tsx": ["OptionName", "OptionHandle"],
  "web/design-system/src/viewer.tsx": ["ViewerRoom", "ViewerSide"],
  "web/design-system/src/editor.tsx": ["SuggestPopup", "FieldBox"],
  "web/design-system/src/emoji-picker.tsx": [
    "PickerFrame",
    "SearchGlyph",
    "PickerGrid",
    "PickerRows",
    "PickerRow",
    "PickerProbe",
  ],
  "web/design-system/src/article-editor.tsx": ["EditorFrame", "ArticlePane", "ModeRow", "PreviewBody"],
  // The fixtures that prove the rules (restrictions.test.js).
  "web/design-system/fixtures/layout.tsx": ["Row", "Shelf", "Gone"],
  "web/design-system/fixtures/viewport.tsx": ["Page", "Column"],
};

/**
 * The frames of a page: the app's, with the parts of it that the viewport rearranges (shell.tsx, ADR-0110 §10), that
 * of a page of its own outside it (form-page.tsx), those of a modal, which under `tablet` is a screen of its own
 * (modal.tsx, and the panel that fills it, dialog.tsx), and that of the article editor's screen (article-editor.tsx).
 * @type {Record<string, string[]>}
 */
export const PAGE_FRAMES = {
  "web/design-system/src/shell.tsx": [
    "Frame",
    "FrameSide",
    "FrameDock",
    "FrameMain",
    "FrameAside",
    "MarkPlace",
    "EntryLabel",
    "NavEntry",
    "FoldRow",
    "CallPart",
  ],
  "web/design-system/src/form-page.tsx": ["FormPage"],
  "web/design-system/src/modal.tsx": ["ModalFrame"],
  "web/design-system/src/dialog.tsx": ["PanelFrame"],
  "web/design-system/src/article-editor.tsx": ["EditorFrame", "ArticlePane", "ModeRow"],
  "web/design-system/src/viewer.tsx": ["ViewerSide"],
  "web/design-system/src/composer.tsx": ["ComposerPanel", "ToolWords", "NoticePlace"],
  "web/design-system/fixtures/viewport.tsx": ["Page"],
};
