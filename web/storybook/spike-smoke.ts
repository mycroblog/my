// SPDX-License-Identifier: AGPL-3.0-only
// Spike #2906, not for main: open the app, the auth entry, and the signed-in home of a demo server in Chromium, and
// print each page's errors, its CSP violations, and a screenshot.
// Usage: bun --bun spike/smoke.ts <origin> <out dir>
import { chromium } from "playwright";

const [origin = "http://localhost:8797", out = "."] = process.argv.slice(2);
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
const errors: string[] = [];
page.on("response", (r) => {
  if (r.status() >= 400) errors.push(`${r.status()} ${r.url()}`);
});
page.on("pageerror", (e) => errors.push(`pageerror: ${e.message}`));
page.on("console", (m) => {
  if (m.type() === "error") errors.push(`console: ${m.text().slice(0, 200)}`);
});
const shot = async (name: string) => {
  await page.waitForTimeout(1500);
  await page.screenshot({ path: `${out}/${name}.png` });
  console.log(JSON.stringify({ name, url: page.url(), title: await page.title(), errors: errors.splice(0) }));
};
await page.goto(`${origin}/`, { waitUntil: "networkidle" });
await shot("landing");
await page.goto(`${origin}/auth/sign_in`, { waitUntil: "networkidle" });
await shot("sign-in");
await page
  .getByLabel(/username|email/i)
  .first()
  .fill("hana@demo.test");
await page
  .getByLabel(/password/i)
  .first()
  .fill("demo-password-hana");
await page
  .getByRole("button", { name: /sign in/i })
  .first()
  .click();
await page.waitForURL((u) => !u.pathname.startsWith("/auth"), { timeout: 20000 });
await page.waitForTimeout(3000);
await shot("home");
const dark = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
console.log(
  JSON.stringify({
    bodyBackground: dark,
    sw: await page.evaluate(() => navigator.serviceWorker.controller?.scriptURL ?? null),
  }),
);
await browser.close();
