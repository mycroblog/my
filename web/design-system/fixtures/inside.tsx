// SPDX-License-Identifier: AGPL-3.0-only
// restrict/design-system-props allows a className and a style inside @my/design-system: no line of this file may fail.
// web/devtools/src/restrictions.test.js lints it; the lint of the package leaves it out.

import { Button } from "@my/design-system";

export const Inside = () => (
  <Button className="p-2" style={{}}>
    Go
  </Button>
);
