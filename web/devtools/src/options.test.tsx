// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { Options } from "./options";

test("Options renders the first variant and lists them all for the overlay", () => {
  const html = renderToStaticMarkup(<Options id="swatch" variants={{ square: <b>square</b>, round: <i>round</i> }} />);
  expect(html).toBe(
    '<div data-my-devtools-options="swatch" data-variant="square" data-variants="[&quot;square&quot;,&quot;round&quot;]" style="display:contents"><b>square</b></div>',
  );
});
