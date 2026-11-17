// SPDX-License-Identifier: AGPL-3.0-only

// Spike #3177: the server's own time to answer cheap read routes under concurrent clients, with no browser.
//
//   bun --conditions=@mycroblog/source web/e2e/src/spike/load.ts <server> [writes-per-hundred]
//
// It signs in as people of the scenario, then for each number of concurrent clients asks the read routes in turn and
// prints the latencies. With writes-per-hundred, that share of the requests posts a status instead.

import { people } from "../scenario";
import { signIn } from "../seed/sign-in";

const [server = "http://localhost:8787", writesArg = "0"] = process.argv.slice(2);
const writes = Number(writesArg);
const reads = ["/instance", "/preferences", "/announcements", "/timelines/home", "/notifications"] as const;

const quantile = (sorted: number[], q: number) => sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * q))] ?? 0;

// People without a second factor, each from an address of their own: an account may make 1,500 requests in five
// minutes (clientapi.AccountLimit), so the requests are spread over them. SPIKE_FROM skips people already used.
const skip = Number(process.env["SPIKE_FROM"] ?? "0");
const signedIn: Awaited<ReturnType<typeof signIn>>[] = [];
for (const [i, p] of people.entries()) {
  if (i < skip || signedIn.length >= 8) continue;
  try {
    signedIn.push(await signIn(server, p, undefined, undefined, `10.77.0.${i + 1}`));
  } catch {
    // A person with a second factor or a passkey: left out.
  }
}
console.log(`signed in: ${signedIn.length} people`);

const steps = process.env["SPIKE_CLIENTS"] ? [Number(process.env["SPIKE_CLIENTS"])] : [1, 4, 12, 24, 48];
for (const clients of steps) {
  const total = Number(process.env["SPIKE_TOTAL"] ?? "300");
  const times: number[] = [];
  const perRoute = new Map<string, number[]>();
  const statuses = new Map<number, number>();
  let next = 0;
  const started = performance.now();
  await Promise.all(
    Array.from({ length: clients }, async (_, c) => {
      const api = signedIn[c % signedIn.length];
      if (api === undefined) throw new Error("no client");
      for (;;) {
        const n = next++;
        if (n >= total) return;
        const write = writes > 0 && n % 100 < writes;
        const route = reads[n % reads.length] ?? "/instance";
        const at = performance.now();
        const res = write
          ? await api.POST("/statuses", {
              params: { header: { "Idempotency-Key": `spike:${started}:${n}` } },
              body: { text: `spike ${n}`, visibility: "public", language: "en" },
            } as never)
          : await api.GET(route as never, {} as never);
        const took = performance.now() - at;
        times.push(took);
        const key = write ? "POST /statuses" : route;
        perRoute.set(key, [...(perRoute.get(key) ?? []), took]);
        const status = res.response.status;
        statuses.set(status, (statuses.get(status) ?? 0) + 1);
      }
    }),
  );
  const wall = performance.now() - started;
  times.sort((a, b) => a - b);
  const routes = [...perRoute]
    .map(([k, v]) => {
      v.sort((a, b) => a - b);
      return `${k} p50 ${quantile(v, 0.5).toFixed(0)}`;
    })
    .join(", ");
  console.log(
    `clients ${String(clients).padStart(2)}: ${total} requests in ${(wall / 1000).toFixed(1)} s (${((total / wall) * 1000).toFixed(0)}/s)` +
      ` | ms p50 ${quantile(times, 0.5).toFixed(0)} p95 ${quantile(times, 0.95).toFixed(0)} p99 ${quantile(times, 0.99).toFixed(0)} max ${(times.at(-1) ?? 0).toFixed(0)}` +
      ` | status ${[...statuses].map(([s, n]) => `${s}x${n}`).join(" ")} | ${routes}`,
  );
}
