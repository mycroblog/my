// SPDX-License-Identifier: AGPL-3.0-only
// restrict/design-system-props: a className or a style in an object spread onto the element, which a prop type misses
// once the object is cast or widened. Each line that must fail says so in an `error:` comment.

import { Button } from "@my/design-system";

const base = { style: { color: "red" } }; // error: <Button> style
const extra = { ...base, className: "p-2" }; // error: <Button> className
const later: Record<string, string> = {};
later["className"] = "p-2"; // error: <Button> className

export const Spread = ({ on }: { on: boolean }) => (
  <div>
    <Button {...(on ? extra : {})}>Go</Button>
    <Button {...later}>Go</Button>
    <Button {...{ style: {} }}>Go</Button> {/* error: <Button> style */}
  </div>
);
