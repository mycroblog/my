// SPDX-License-Identifier: AGPL-3.0-only
// What the screen stories of voice chat share: the server with voice chat on, and the states of a request.
import { delay, http, HttpResponse, type RequestHandler } from "msw";
import { expect, waitFor } from "storybook/test";
import { instanceFixture } from "../../../test/fixtures";
import { apiBase, problem } from "../../../test/msw/http";

/** The instance with voice chat on, where every user may open a room. */
export const voiceOn: RequestHandler = http.get(`${apiBase}/instance`, () =>
  HttpResponse.json({ ...instanceFixture, voice_enabled: true, voice_room_starters: "everyone" }),
);

/** A GET of `path` that never answers. */
export function neverAnswering(path: string): RequestHandler {
  return http.get(`${apiBase}${path}`, async () => {
    await delay("infinite");
    return new HttpResponse(null, { status: 204 });
  });
}

/** A GET of `path` that fails with a 500, as a server that broke. */
export function failing(path: string): RequestHandler {
  return http.get(`${apiBase}${path}`, () => problem(500, "about:blank", "Internal Server Error"));
}

/** Waits for the page's heading over a busy region that has the loading's name, and no spinner (G15). */
export async function showsSkeleton({ canvasElement }: { canvasElement: HTMLElement }, label: string): Promise<void> {
  await waitFor(
    () => {
      if (canvasElement.querySelector("main h1") === null) throw new Error("the page has no h1 yet");
      if (canvasElement.querySelector("main [aria-busy='true']") === null) throw new Error("nothing is busy yet");
    },
    { timeout: 10_000 },
  );
  await expect(canvasElement.querySelector("main [aria-busy='true']")).toHaveTextContent(label);
  await expect(canvasElement.querySelector("main [role='progressbar']")).toBeNull();
}
