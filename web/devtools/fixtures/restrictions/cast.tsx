// SPDX-License-Identifier: AGPL-3.0-only
// restrict/design-system-props: a className or a style let through by a cast of the value or of the component. Each
// line that must fail says so in an `error:` comment.

import type { ComponentType } from "react";
import { Button } from "@my/design-system";

const Loose = Button as unknown as ComponentType<{ className: string }>;
const props = { className: "p-2" } as object; // error: <Button> className

export const Cast = () => (
  <div>
    <Loose className="p-2" /> {/* error: <Loose> className */}
    <Button {...props}>Go</Button>
    <Button {...({ style: {} } satisfies object)}>Go</Button> {/* error: <Button> style */}
  </div>
);
