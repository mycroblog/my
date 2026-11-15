// SPDX-License-Identifier: AGPL-3.0-only
// What a hook returns now, from what the cache holds: one render on the server, with no DOM and no effects.
import { renderToStaticMarkup } from "react-dom/server";
import type { AppData } from "../app-data";
import { AppDataProvider } from "../provider";

/** The value `hook` returns in a component under `data`, rendered once. */
export function probe<T>(data: AppData, hook: () => T): T {
  const seen: { value?: T } = {};
  function Probe() {
    seen.value = hook();
    return null;
  }
  renderToStaticMarkup(
    <AppDataProvider data={data}>
      <Probe />
    </AppDataProvider>,
  );
  if (!("value" in seen)) throw new Error("the probe did not render");
  return seen.value;
}
