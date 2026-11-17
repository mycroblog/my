// SPDX-License-Identifier: AGPL-3.0-only
// A story may give an element of the DOM a class and a style, to stand a specimen in a width; a part of the design
// system takes neither there too.

import { Box } from "@my/design-system";

export const Specimen = () => (
  <div style={{ maxInlineSize: "20rem" }}>
    <Box className="text-caption" /> {/* error: <Box> className */}
  </div>
);
