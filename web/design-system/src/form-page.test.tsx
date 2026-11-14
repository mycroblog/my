// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { Button, LinkButton } from "./button";
import {
  CheckField,
  FormActions,
  FormAlert,
  FormChoice,
  FormPage,
  FormPageFooter,
  TextField,
  WidgetSlot,
} from "./form-page";

const wordmark = { light: "/light.svg", dark: "/dark.svg" };

test("the page shows the server's introduction from the desktop width, which stays in view, and its mark in the column below it", () => {
  const html = renderToStaticMarkup(
    <FormPage
      server={{ name: "Hanami", banner: "/banner.png", description: "A town square." }}
      wordmark={wordmark}
      footer={<FormPageFooter label="Policies" links={[{ href: "/about/terms", label: "Terms" }]} />}
    >
      <h1>Sign in</h1>
    </FormPage>,
  );
  expect(html).toMatch(
    /<aside class="hidden desktop:sticky desktop:top-0 [^"]*desktop:h-dvh[^"]*" aria-label="Hanami">/,
  );
  expect(html).toContain('src="/banner.png"');
  // Without a logo of its own, the server shows the built-in wordmark, for light and dark, in both places.
  expect(html.match(/src="\/light.svg"/g)).toHaveLength(2);
  expect(html.match(/src="\/dark.svg"/g)).toHaveLength(2);
  expect(html).toMatch(/<header class="desktop:hidden">/);
  expect(html).toMatch(/<main [^>]*><h1>Sign in<\/h1><\/main>/);
  expect(html).toMatch(/<nav aria-label="Policies">[^]*href="\/about\/terms"/);
  expect(renderToStaticMarkup(<FormPageFooter label="Policies" links={[]} />)).toBe("");
});

test("a server with its own logo shows it in place of the wordmark; with no banner the introduction sits in the middle", () => {
  const html = renderToStaticMarkup(
    <FormPage server={{ name: "Hanami", logo: "/logo.svg", logoDark: "/logo-dark.svg" }} wordmark={wordmark}>
      x
    </FormPage>,
  );
  expect(html).not.toContain("/light.svg");
  expect(html).toContain('src="/logo-dark.svg"');
  expect(html).toContain("desktop:items-center");
});

test("a field's hint and error are read with it, and the error marks it", () => {
  const html = renderToStaticMarkup(
    <TextField id="name" name="username" label="Username" hint="Up to 30." error="Taken." describedBy="prompt" />,
  );
  expect(html).toContain('<label class="font-label" for="name">Username</label>');
  const input = /<input [^>]*>/.exec(html)?.[0] ?? "";
  for (const want of [
    'id="name"',
    'name="username"',
    'aria-invalid="true"',
    'aria-describedby="prompt name-hint name-error"',
  ])
    expect(input).toContain(want);
  expect(html.indexOf('id="name"')).toBeLessThan(html.indexOf('id="name-error"'));
  expect(renderToStaticMarkup(<TextField id="n" name="n" label="N" />)).not.toContain("aria-describedby");
  expect(renderToStaticMarkup(<TextField id="n" name="n" label="N" multiline />)).toContain("<textarea");
});

test("a checkbox in error says so under it and is marked", () => {
  const html = renderToStaticMarkup(
    <CheckField id="agree" name="agreement" value="yes" label="I agree" error="Agree first." describedBy="docs" />,
  );
  const input = /<input [^>]*>/.exec(html)?.[0] ?? "";
  for (const want of [
    'type="checkbox"',
    'name="agreement"',
    'value="yes"',
    'aria-invalid="true"',
    'aria-describedby="docs agree-error"',
  ])
    expect(input).toContain(want);
  expect(html.indexOf("<input")).toBeLessThan(html.indexOf('id="agree-error"'));
});

test("a form's alert is announced, knocked out with no hue, and marked to take the focus only when asked", () => {
  const html = renderToStaticMarkup(
    <FormActions>
      <FormAlert focus>Refused.</FormAlert>
      <Button kind="primary" type="submit">
        Send
      </Button>
    </FormActions>,
  );
  expect(html).toMatch(
    /<p class="[^"]*bg-text[^"]*text-surface[^"]*" role="alert" tabindex="-1" data-focus-on-load="">/,
  );
  expect(html).not.toMatch(/class="[^"]*accent[^"]*" role="alert"/);
  expect(html).toMatch(/Refused\.<\/span><\/p><button/);
  expect(renderToStaticMarkup(<FormAlert>Refused.</FormAlert>)).not.toContain("data-focus-on-load");
});

test("a choice keeps its children's order, the refusal at the start; a link button stays a link", () => {
  const html = renderToStaticMarkup(
    <FormChoice>
      <Button>Deny</Button>
      <Button kind="primary">Allow</Button>
    </FormChoice>,
  );
  expect(html.indexOf("Deny")).toBeLessThan(html.indexOf("Allow"));
  expect(renderToStaticMarkup(<LinkButton href="/start">GitHub</LinkButton>)).toMatch(/^<a [^>]*href="\/start"/);
});

test("a widget's place carries the service's class and data attributes", () => {
  expect(renderToStaticMarkup(<WidgetSlot marker="cf-turnstile" data={{ "data-sitekey": "k" }} />)).toMatch(
    /<div class="cf-turnstile [^"]*" data-sitekey="k"><\/div>/,
  );
});
