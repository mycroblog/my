// SPDX-License-Identifier: AGPL-3.0-only
// Above the design system no element gets a class or a style (ADR-0025 §4): a part of this layer composes the parts
// of @my/design-system by their props.

import { Box, Stack } from "@my/design-system";

const given = { className: "text-caption" }; // error: <div> className

export const Classes = () => (
  <Stack gap="entry-line" surface="raised" radius="control" pad="embed">
    <Box as="p" text="caption" tone="muted">
      Composed
    </Box>
    <p className="text-caption">Written</p> {/* error: <p> className */}
    <span style={{ color: "inherit" }}>Styled</span> {/* error: <span> style */}
    <div {...given} />
    <Box className="text-caption" /> {/* error: <Box> className */}
  </Stack>
);
