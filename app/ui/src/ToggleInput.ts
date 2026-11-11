// SPDX-License-Identifier: AGPL-3.0-only
// The native input of a checkbox, radio or switch, laid over its drawing.
//
// React Aria's CheckboxButton, RadioButton and SwitchButton render a label whose first child is a visually hidden
// span (inline styles: 1px, clipped, absolute) around the native input, then the drawing and the text. The clipped
// input sits under the drawing, so a press aimed at the input itself, the control of the accessibility tree, meets the
// drawing instead: Playwright refuses the click ("intercepts pointer events"), and so does the L4 agent, which acts on
// the ARIA snapshot. These classes stretch that span over the drawing at the label's start, unclipped and transparent,
// so that the input is what a pointer on the box, circle or track hits; the text stays the label's own. The `!`
// overrides the inline styles of React Aria's VisuallyHidden.

/** The label of the control: positions the span of the native input and the input in it. */
const base = [
  "relative",
  "[&>span:has(>input)]:start-0 [&>span:has(>input)]:top-1/2 [&>span:has(>input)]:z-10 [&>span:has(>input)]:-translate-y-1/2",
  "[&>span:has(>input)]:m-0! [&>span:has(>input)]:[clip:auto]! [&>span:has(>input)]:[clip-path:none]!",
  "[&>span>input]:m-0 [&>span>input]:block [&>span>input]:size-full [&>span>input]:cursor-[inherit] [&>span>input]:opacity-0",
];

/** For a checkbox or radio: the input over the 1.25rem box or circle. */
export const inputOverBox = [...base, "[&>span:has(>input)]:size-5!"];

/** For a switch: the input over the 2.75rem × 1.625rem track. */
export const inputOverTrack = [...base, "[&>span:has(>input)]:h-6.5! [&>span:has(>input)]:w-11!"];
