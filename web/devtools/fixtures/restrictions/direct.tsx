// SPDX-License-Identifier: AGPL-3.0-only
// restrict/design-system-props: a className or a style written on the element. Each line that must fail says so in an
// `error:` comment; restrictions.test.js lints this file and compares.

import { Avatar, Button } from "@my/design-system";

export const Direct = () => (
  <div>
    <Button className="p-2">Go</Button> {/* error: <Button> className */}
    <Avatar style={{ inlineSize: 40 }} /> {/* error: <Avatar> style */}
  </div>
);
