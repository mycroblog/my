// SPDX-License-Identifier: MIT
import { all, createClient, NetworkError, ProblemError, staticToken, uploadMedia } from "@mycroblog/mykit";

// A client of one server. The origin has no path: the client API is under /api/v1.
const api = createClient({
  server: "https://example.com",
  tokenSource: staticToken("access token"), // see oauth.ts for a token that renews itself
  acceptLanguage: "en",
});

// Post a status. The library adds an Idempotency-Key that it keeps for every retry, so a post that is sent again
// after a 429 or a 503 is applied once. A call that gets an answer of 400 or more rejects with a ProblemError.
export async function post(text: string): Promise<string> {
  try {
    const { data } = await api.POST("/statuses", { body: { text, visibility: "public" } });
    if (data === undefined) throw new Error("the answer has no body");
    return data.id;
  } catch (error) {
    if (error instanceof ProblemError) {
      // Branch on type and status, never on title or detail, which the server localizes.
      console.error(error.status, error.type);
    } else if (error instanceof NetworkError) {
      console.error("no answer", error.cause);
    }
    throw error;
  }
}

// Iterate the home timeline page by page; the next page is fetched only when the loop needs it.
export async function printHomeTimeline(limit: number): Promise<void> {
  let seen = 0;
  for await (const status of all(async (cursor) => {
    const { data } = await api.GET("/timelines/home", { params: { query: cursor === undefined ? {} : { cursor } } });
    if (data === undefined) throw new Error("the answer has no body");
    return data;
  })) {
    console.log(status.id);
    if (++seen >= limit) break; // a timeline has no end
  }
}

// Upload a file; the answer is usually still processing, so poll GET /media/{id} until it is ready.
export async function upload(photo: Blob): Promise<string> {
  const media = await uploadMedia(api, { file: photo, fileName: "photo.jpg", description: "A photo of a cat" });
  return media.id;
}
