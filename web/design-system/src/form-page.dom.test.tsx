// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button, TextButton } from "./button";
import {
  CheckField,
  FieldError,
  Form,
  FormAccount,
  FormActions,
  FormAlert,
  FormBadge,
  FormCaution,
  FormChoice,
  FormHead,
  FormLink,
  FormLinks,
  FormList,
  FormOthers,
  FormPage,
  FormPageFooter,
  FormSection,
  FormText,
  GlyphLabel,
  TextField,
  WidgetSlot,
  focusOnLoadAttribute,
} from "./form-page";

const wordmark = { light: "/light.svg", dark: "/dark.svg" };

/** The addresses of the images that say nothing to assistive technology, in the document's order. */
const decorations = () => screen.queryAllByRole("presentation").map((image) => image.getAttribute("src"));

/** Whether `later` comes after `earlier` in the reading order. */
const follows = (later: Element, earlier: Element) =>
  (earlier.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

test("a page has the server's introduction under its name, the screen as its main part, and the footer's links under theirs", () => {
  render(
    <FormPage
      server={{ name: "Hanami", banner: "/banner.png", description: "A town square." }}
      wordmark={wordmark}
      footer={<FormPageFooter label="Policies" links={[{ href: "/about/terms", label: "Terms" }]} />}
    >
      <FormHead title="Sign in" />
    </FormPage>,
  );

  const introduction = screen.getByRole("complementary", { name: "Hanami" });
  expect(within(introduction).getByText("A town square.")).not.toBeNull();
  expect(within(screen.getByRole("banner")).getByText("Hanami")).not.toBeNull();
  expect(within(screen.getByRole("main")).getByRole("heading", { level: 1, name: "Sign in" })).not.toBeNull();
  const policies = within(screen.getByRole("contentinfo")).getByRole("navigation", { name: "Policies" });
  expect(within(policies).getByRole("link", { name: "Terms" }).getAttribute("href")).toBe("/about/terms");
  // The banner and the mark say nothing the name beside them does not: no image is read.
  expect(decorations()).toContain("/banner.png");
  expect(screen.queryAllByRole("img")).toEqual([]);
});

test("a server without a logo shows the built-in wordmark, and one with its own shows that in its place", () => {
  const { unmount } = render(
    <FormPage server={{ name: "Hanami" }} wordmark={wordmark}>
      <FormHead title="Sign in" />
    </FormPage>,
  );
  expect(new Set(decorations())).toEqual(new Set(["/light.svg", "/dark.svg"]));
  unmount();

  render(
    <FormPage server={{ name: "Hanami", logo: "/logo.svg", logoDark: "/logo-dark.svg" }} wordmark={wordmark}>
      <FormHead title="Sign in" />
    </FormPage>,
  );
  expect(new Set(decorations())).toEqual(new Set(["/logo.svg", "/logo-dark.svg"]));
  expect(screen.queryAllByRole("img")).toEqual([]);
});

test("the server's front page is named by the server, with its introduction and what follows it", () => {
  render(
    <FormPage
      server={{ name: "Hanami", description: "A town square." }}
      wordmark={wordmark}
      front={{ more: <a href="/about">About this server</a> }}
    >
      <FormText>Sign in to join.</FormText>
    </FormPage>,
  );

  const header = screen.getByRole("banner");
  expect(within(header).getByRole("heading", { level: 1, name: "Hanami" })).not.toBeNull();
  const description = within(header).getByText("A town square.");
  const more = within(header).getByRole("link", { name: "About this server" });
  expect(follows(more, description)).toBe(true);
  // The introduction is the page's header: there is none beside the column.
  expect(screen.queryByRole("complementary")).toBeNull();
  expect(within(screen.getByRole("main")).getByText("Sign in to join.")).not.toBeNull();
});

test("a page without a server is the screen alone", () => {
  render(
    <FormPage wordmark={wordmark}>
      <FormHead title="Back soon" />
    </FormPage>,
  );

  expect(within(screen.getByRole("main")).getByRole("heading", { level: 1, name: "Back soon" })).not.toBeNull();
  expect(screen.queryByRole("complementary")).toBeNull();
  expect(screen.queryByRole("banner")).toBeNull();
  expect(decorations()).toEqual([]);
});

test("a footer without links is nothing", () => {
  render(<FormPageFooter label="Policies" links={[]} />);

  expect(screen.queryByRole("navigation")).toBeNull();
});

test("a screen's head is its title as the page's h1, over the lines under it", () => {
  render(
    <FormHead title="Reset your password">
      <FormText muted>We will send you a link.</FormText>
    </FormHead>,
  );

  const title = screen.getByRole("heading", { level: 1, name: "Reset your password" });
  expect(follows(screen.getByText("We will send you a link."), title)).toBe(true);
});

test("a paragraph that is an outcome is a status, and one that is not is none", () => {
  render(
    <>
      <FormText role="status">We sent you a link.</FormText>
      <FormText>Check your inbox.</FormText>
    </>,
  );

  expect(screen.getByRole("status").textContent).toBe("We sent you a link.");
  expect(screen.getByText("Check your inbox.").hasAttribute("role")).toBe(false);
});

test("a form posts its fields to its action: a field under its name, a checkbox its value only while checked", async () => {
  const user = userEvent.setup();
  const posts: { action: string | null; method: string; fields: [string, FormDataEntryValue][] }[] = [];
  render(
    <Form
      action="/auth/sign-up"
      onSubmit={(event) => {
        event.preventDefault();
        const form = event.currentTarget;
        posts.push({
          action: form.getAttribute("action"),
          method: form.method,
          fields: [...new FormData(form).entries()],
        });
      }}
    >
      <TextField id="username" name="username" label="Username" />
      <TextField id="bio" name="bio" label="Bio" multiline defaultValue="Hello." />
      <CheckField id="agree" name="agreement" value="yes" label="I agree to the terms" />
      <FormActions>
        <Button kind="primary" type="submit">
          Sign up
        </Button>
      </FormActions>
    </Form>,
  );

  await user.type(screen.getByRole("textbox", { name: "Username" }), "sam");
  await user.click(screen.getByRole("button", { name: "Sign up" }));
  await user.click(screen.getByRole("checkbox", { name: "I agree to the terms" }));
  await user.click(screen.getByRole("button", { name: "Sign up" }));

  expect(posts).toEqual([
    {
      action: "/auth/sign-up",
      method: "post",
      fields: [
        ["username", "sam"],
        ["bio", "Hello."],
      ],
    },
    {
      action: "/auth/sign-up",
      method: "post",
      fields: [
        ["username", "sam"],
        ["bio", "Hello."],
        ["agreement", "yes"],
      ],
    },
  ]);
});

test("a form's ref reaches the form, to send it from a script", () => {
  const reached: HTMLFormElement[] = [];
  render(
    <Form
      action="/auth/passkey"
      ref={(form) => {
        if (form) reached.push(form);
      }}
    >
      <TextField id="answer" name="answer" label="Answer" defaultValue="signed" />
    </Form>,
  );

  expect(reached.map((form) => [...new FormData(form).entries()])).toEqual([[["answer", "signed"]]]);
});

test("a text field is a textbox named by its label alone, and reports its value as the reader types", async () => {
  const user = userEvent.setup();
  const values: string[] = [];
  render(
    <TextField
      id="display"
      name="display"
      label="Display name"
      end={<a href="/help/names">What is this?</a>}
      defaultValue="Sa"
      onChange={(value) => values.push(value)}
    />,
  );
  const field = screen.getByRole<HTMLInputElement>("textbox", { name: "Display name" });
  expect(field.value).toBe("Sa");
  expect(field.getAttribute("aria-invalid")).not.toBe("true");
  expect(field.hasAttribute("aria-describedby")).toBe(false);
  expect(screen.getByRole("link", { name: "What is this?" }).getAttribute("href")).toBe("/help/names");

  await user.type(field, "m!");

  expect(values).toEqual(["Sam", "Sam!"]);
});

test("a text field's hint and error describe it, after what the caller says describes it, and the error marks it invalid", () => {
  render(
    <>
      <p id="prompt">Choose how others find you.</p>
      <TextField
        id="name"
        name="username"
        label="Username"
        hint="Up to 30 letters."
        error="That name is taken."
        describedBy="prompt"
      />
    </>,
  );

  const field = screen.getByRole("textbox", {
    name: "Username",
    description: "Choose how others find you. Up to 30 letters. That name is taken.",
  });
  expect(field.getAttribute("aria-invalid")).toBe("true");
  // The error is right under the field: it is read after it.
  expect(follows(screen.getByText("That name is taken."), field)).toBe(true);
});

test("a text field of the search type is a searchbox, and one of several lines takes a line break", async () => {
  const user = userEvent.setup();
  render(
    <>
      <TextField id="find" name="q" label="Find a member" type="search" />
      <TextField id="bio" name="bio" label="Bio" multiline />
    </>,
  );

  expect(screen.getByRole("searchbox", { name: "Find a member" })).not.toBeNull();
  const bio = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Bio" });
  await user.type(bio, "One{Enter}Two");
  expect(bio.value).toBe("One\nTwo");
});

test("a text field carries what the browser and the keyboard need of it: its kind, autofill, its limits", async () => {
  const user = userEvent.setup();
  render(
    <>
      <TextField
        id="email"
        name="email"
        label="Email"
        type="email"
        autoComplete="email"
        inputMode="email"
        autoCapitalize="none"
        required
      />
      <TextField
        id="password"
        name="password"
        label="Password"
        type="password"
        autoComplete="new-password"
        minLength={8}
      />
      <TextField id="code" name="code" label="Code" maxLength={3} />
      <TextField id="handle" name="handle" label="Handle" defaultValue="@sam" readOnly />
    </>,
  );

  const email = screen.getByRole<HTMLInputElement>("textbox", { name: "Email" });
  expect(email.type).toBe("email");
  expect(email.required).toBe(true);
  expect(email.getAttribute("autocomplete")).toBe("email");
  expect(email.getAttribute("inputmode")).toBe("email");
  expect(email.getAttribute("autocapitalize")).toBe("none");
  // A password is no textbox to assistive technology: its label finds it.
  const password = screen.getByLabelText<HTMLInputElement>("Password");
  expect(password.type).toBe("password");
  expect(password.getAttribute("autocomplete")).toBe("new-password");
  expect(password.minLength).toBe(8);
  const code = screen.getByRole<HTMLInputElement>("textbox", { name: "Code" });
  await user.type(code, "12345");
  expect(code.value).toBe("123");
  const handle = screen.getByRole<HTMLInputElement>("textbox", { name: "Handle" });
  expect(handle.readOnly).toBe(true);
  await user.type(handle, "x");
  expect(handle.value).toBe("@sam");
});

test("a text field that asks for the focus has it as it mounts", () => {
  render(<TextField id="list" name="title" label="List name" autoFocus />);

  expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "List name" }));
});

test("a field's error describes the field that names its id", () => {
  render(
    <>
      <input aria-label="Date of birth" aria-describedby="birth-error" />
      <FieldError id="birth-error">Enter a date in the past.</FieldError>
    </>,
  );

  expect(
    screen.getByRole("textbox", { name: "Date of birth", description: "Enter a date in the past." }),
  ).not.toBeNull();
});

test("a check field is a checkbox named by its label, and Space or a press on the label turns it", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(
    <CheckField
      id="agree"
      name="agreement"
      value="yes"
      label="I agree to the terms"
      required
      onChange={(checked) => changes.push(checked)}
    />,
  );
  const box = screen.getByRole<HTMLInputElement>("checkbox", { name: "I agree to the terms", checked: false });
  expect(box.required).toBe(true);
  expect(box.getAttribute("aria-invalid")).not.toBe("true");

  await user.tab();
  expect(document.activeElement).toBe(box);
  await user.keyboard(" ");
  expect(screen.getByRole("checkbox", { name: "I agree to the terms", checked: true })).toBe(box);
  await user.click(screen.getByText("I agree to the terms"));
  expect(screen.getByRole("checkbox", { name: "I agree to the terms", checked: false })).toBe(box);

  expect(changes).toEqual([true, false]);
});

test("a check field starts checked when its default says so, and one its owner keeps stays as the owner says", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(
    <>
      <CheckField id="multiple" name="multiple" value="1" label="Allow several choices" defaultChecked />
      <CheckField
        id="hide"
        name="hide"
        value="1"
        label="Hide the totals"
        checked={false}
        onChange={(checked) => changes.push(checked)}
      />
    </>,
  );
  expect(screen.getByRole("checkbox", { name: "Allow several choices", checked: true })).not.toBeNull();

  await user.click(screen.getByRole("checkbox", { name: "Hide the totals" }));

  expect(changes).toEqual([true]);
  expect(screen.getByRole("checkbox", { name: "Hide the totals", checked: false })).not.toBeNull();
});

test("a check field's error describes it, after what the caller says describes it, and marks it invalid", () => {
  render(
    <>
      <p id="documents">Read the terms and the privacy policy.</p>
      <CheckField
        id="agree"
        name="agreement"
        value="yes"
        label="I agree"
        error="Agree to go on."
        describedBy="documents"
      />
    </>,
  );

  const box = screen.getByRole("checkbox", {
    name: "I agree",
    description: "Read the terms and the privacy policy. Agree to go on.",
  });
  expect(box.getAttribute("aria-invalid")).toBe("true");
  expect(follows(screen.getByText("Agree to go on."), box)).toBe(true);
});

test("a form's alert is an alert that says what was refused and holds what the reader may do about it", async () => {
  const user = userEvent.setup();
  let opened = 0;
  render(
    <FormActions>
      <FormAlert regular action={<TextButton onPress={() => opened++}>Open the newer draft</TextButton>}>
        A newer draft replaced this one.
      </FormAlert>
      <Button kind="primary" type="submit">
        Save
      </Button>
    </FormActions>,
  );

  const alert = screen.getByRole("alert");
  expect(alert.textContent).toContain("A newer draft replaced this one.");
  // It says so before the button that sent the form.
  expect(follows(screen.getByRole("button", { name: "Save" }), alert)).toBe(true);
  await user.click(within(alert).getByRole("button", { name: "Open the newer draft" }));
  expect(opened).toBe(1);
});

test("only a form's alert that asks for the focus is marked for the page to move it there", () => {
  render(
    <>
      <FormAlert focus>The password is wrong.</FormAlert>
      <FormAlert>Too many attempts.</FormAlert>
    </>,
  );

  const marked = screen.getAllByRole("alert").filter((alert) => alert.hasAttribute(focusOnLoadAttribute));
  expect(marked.map((alert) => alert.textContent)).toEqual(["The password is wrong."]);
});

test("a choice keeps its order: Tab reaches the refusal, then the confirmation", async () => {
  const user = userEvent.setup();
  render(
    <FormChoice>
      <Button>Deny</Button>
      <Button kind="primary">Allow</Button>
    </FormChoice>,
  );

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Deny" }));
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Allow" }));
});

test("the other ways to go on are a region named by its heading, and a glyph adds nothing to a button's name", () => {
  render(
    <FormOthers heading="Or sign in with">
      <Button>
        <GlyphLabel icon="passkey">A passkey</GlyphLabel>
      </Button>
    </FormOthers>,
  );

  const others = screen.getByRole("region", { name: "Or sign in with" });
  expect(within(others).getByRole("heading", { level: 2, name: "Or sign in with" })).not.toBeNull();
  expect(within(others).getByRole("button", { name: "A passkey" })).not.toBeNull();
  expect(screen.queryAllByRole("img")).toEqual([]);
});

test("a screen's link leads to its href, and one that opens a new tab says so and gives the new page no way back", () => {
  render(
    <FormLinks>
      <FormLink href="/auth/sign-up">Create an account</FormLink>
      <FormLink href="https://status.example/" newTab="(opens in a new tab)">
        Status
      </FormLink>
    </FormLinks>,
  );

  const here = screen.getByRole<HTMLAnchorElement>("link", { name: "Create an account" });
  expect(here.getAttribute("href")).toBe("/auth/sign-up");
  expect(here.target).toBe("");
  const away = screen.getByRole<HTMLAnchorElement>("link", { name: "Status (opens in a new tab)" });
  expect(away.getAttribute("href")).toBe("https://status.example/");
  expect(away.target).toBe("_blank");
  expect(away.rel.split(" ")).toContain("noopener");
});

test("a section is a region named by its heading, a badge beside it, over its list", () => {
  render(
    <FormSection
      id="suspension"
      heading={
        <>
          Suspension <FormBadge>In effect</FormBadge>
        </>
      }
    >
      <FormList items={["You cannot post.", "You cannot follow."]} />
    </FormSection>,
  );

  const section = screen.getByRole("region", { name: "Suspension In effect" });
  expect(within(section).getByRole("heading", { level: 2, name: "Suspension In effect" })).not.toBeNull();
  const items = within(within(section).getByRole("list")).getAllByRole("listitem");
  expect(items.map((item) => item.textContent)).toEqual(["You cannot post.", "You cannot follow."]);
});

test("a caution and an account say their line, and their glyph and avatar say nothing", () => {
  render(
    <>
      <FormCaution strong>This app is not the one you signed in to before.</FormCaution>
      <FormAccount name="Sam">Signed in as Sam</FormAccount>
    </>,
  );

  expect(screen.getByText("This app is not the one you signed in to before.")).not.toBeNull();
  expect(screen.getByText("Signed in as Sam")).not.toBeNull();
  expect(screen.queryAllByRole("img")).toEqual([]);
});

test("a widget's place is what its service looks for: an element of its class with the data it reads", () => {
  render(<WidgetSlot markerClassName="cf-turnstile" data={{ "data-sitekey": "k", "data-theme": "auto" }} />);

  // A third party's script finds its place by this class, so the test does too.
  const places = Array.from(document.getElementsByClassName("cf-turnstile"), (place) => [
    place.getAttribute("data-sitekey"),
    place.getAttribute("data-theme"),
  ]);
  expect(places).toEqual([["k", "auto"]]);
});
