// SPDX-License-Identifier: AGPL-3.0-only
// restrict/design-system-props: a component of @my/design-system under another name in the same file: renamed on
// import, bound again, destructured from the namespace, a member of it, or from a subpath. Each line that must fail says
// so in an `error:` comment.

import { Button as Action } from "@my/design-system";
import * as ui from "@my/design-system";
import { Preview } from "@my/design-system/preview";

const Primary = Action;
const { Avatar: Picture } = ui;

export const Renamed = () => (
  <Preview>
    <Action className="a">Go</Action> {/* error: <Action> className */}
    <Primary style={{}}>Go</Primary> {/* error: <Primary> style */}
    <Picture className="c" /> {/* error: <Picture> className */}
    <ui.Button className="d">Go</ui.Button> {/* error: <ui.Button> className */}
  </Preview>
);

export const Shown = () => <Preview style={{}} />; // error: <Preview> style
