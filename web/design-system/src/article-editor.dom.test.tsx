// SPDX-License-Identifier: AGPL-3.0-only

// What the article editor's parts are to assistive technology, to the keyboard and to their caller: the screen's
// heading and the order it is read in, each field's name, state and error, what the hashtags' field does with what is
// typed, and what a press asks of the caller. Which pane shows at which width is a layout test's; the arrow keys of
// Write and Preview are in article-editor.jsdom.test.tsx.

import { expect, spyOn, test } from "bun:test";
import { Button } from "@my/design-system";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import {
  ArticleDetails,
  ArticleField,
  ArticleFrame,
  ArticleLimits,
  ArticleMode,
  ArticlePreview,
  ArticleTextArea,
  ArticleTitleInput,
  CoverAdd,
  CoverRemove,
  CoverRow,
  TagInput,
} from "./article-editor";

const none = () => undefined;

/** Whether each of `elements` follows the one before it in the document, the order a screen reader reads them in. */
const inOrder = (...elements: Element[]) =>
  elements.every((element, i) => {
    const before = elements[i - 1];
    return before === undefined || (before.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
  });

test("the screen is headed by its title, and read in order: the head, what is written, the preview, the limits, the note, the error, the row", () => {
  render(
    <ArticleFrame
      title="New article"
      closeLabel="Close"
      onClose={none}
      headEnd={<Button>Drafts</Button>}
      preview={{ caption: "As it will be published", children: <p>The preview</p> }}
      limits={<p>12,345 of 100,000 bytes</p>}
      note={<p>Scheduled for Monday</p>}
      alert={<p role="alert">The article could not be published.</p>}
      tools={<Button>Save draft</Button>}
      action={<Button>Publish</Button>}
    >
      <textarea aria-label="Body" />
    </ArticleFrame>,
  );

  expect(
    inOrder(
      screen.getByRole("heading", { level: 1, name: "New article" }),
      screen.getByRole("button", { name: "Drafts" }),
      screen.getByRole("button", { name: "Close" }),
      screen.getByRole("textbox", { name: "Body" }),
      screen.getByText("The preview"),
      screen.getByText("12,345 of 100,000 bytes"),
      screen.getByText("Scheduled for Monday"),
      screen.getByRole("alert"),
      screen.getByRole("button", { name: "Save draft" }),
      screen.getByRole("button", { name: "Publish" }),
    ),
  ).toBe(true);
  expect(screen.getByText("As it will be published")).not.toBeNull();
});

test("the screen's close asks to leave, and without a preview it has no caption of one", async () => {
  const user = userEvent.setup();
  let closed = 0;
  render(
    <ArticleFrame title="Edit article" closeLabel="Close" onClose={() => closed++}>
      <textarea aria-label="Body" />
    </ArticleFrame>,
  );

  await user.click(screen.getByRole("button", { name: "Close" }));

  expect(closed).toBe(1);
  expect(screen.getByRole("heading", { level: 1, name: "Edit article" })).not.toBeNull();
  expect(screen.queryByText("As it will be published")).toBeNull();
});

const modes = [
  { id: "write", name: "Write" },
  { id: "preview", name: "Preview" },
] as const;

/** Write or Preview as a caller holds it, and each mode it was asked for. */
function Modes({ asked }: { asked: string[] }) {
  const [current, setCurrent] = useState<"write" | "preview">("write");
  return (
    <>
      <ArticleMode
        label="View"
        items={modes}
        current={current}
        onChange={(id) => {
          asked.push(id);
          setCurrent(id);
        }}
      />
      <button type="button">After</button>
    </>
  );
}

test("Write or Preview is a group of radios named by its label, the current one checked, and a press asks for the other", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Modes asked={asked} />);
  const group = screen.getByRole("radiogroup", { name: "View" });

  expect(within(group).getByRole("radio", { name: "Write", checked: true })).not.toBeNull();
  await user.click(within(group).getByRole("radio", { name: "Preview", checked: false }));

  expect(asked).toEqual(["preview"]);
  expect(within(group).getByRole("radio", { name: "Preview", checked: true })).not.toBeNull();
  expect(within(group).getByRole("radio", { name: "Write", checked: false })).not.toBeNull();
});

test("Write or Preview is one Tab stop, on the current one; an arrow key goes to the other, and Space or Enter chooses it", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Modes asked={asked} />);

  await user.tab();
  expect(document.activeElement === screen.getByRole("radio", { name: "Write" })).toBe(true);
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement === screen.getByRole("radio", { name: "Preview" })).toBe(true);
  // The focus alone chooses nothing.
  expect(asked).toEqual([]);
  await user.keyboard(" ");
  expect(asked).toEqual(["preview"]);
  await user.keyboard("{Enter}");
  expect(asked).toEqual(["preview", "preview"]);

  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "After" })).toBe(true);
});

test("the title is a field named by its label, of one line, with its count at its end", async () => {
  const user = userEvent.setup();
  const typed: string[] = [];
  function Title() {
    const [value, setValue] = useState("Sourdough");
    return (
      <ArticleTitleInput
        id="title"
        label="Title"
        placeholder="Title"
        value={value}
        onChange={(next) => {
          typed.push(next);
          setValue(next);
        }}
        count={<span>3 characters left</span>}
      />
    );
  }
  render(<Title />);
  const title = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Title" });

  expect(title.value).toBe("Sourdough");
  expect(title.placeholder).toBe("Title");
  expect(title.getAttribute("aria-invalid")).toBeNull();
  await user.type(title, "!{Enter}");

  // Enter writes no line break in a title.
  expect(typed).toEqual(["Sourdough!"]);
  expect(inOrder(title, screen.getByText("3 characters left"))).toBe(true);
});

test("the title says what is wrong with it under it, which describes it", () => {
  render(
    <ArticleTitleInput id="title" label="Title" placeholder="Title" value="" onChange={none} error="Write a title." />,
  );

  const title = screen.getByRole("textbox", { name: "Title", description: "Write a title." });
  expect(title.getAttribute("aria-invalid")).toBe("true");
  expect(inOrder(title, screen.getByText("Write a title."))).toBe(true);
});

test("a read-only title and a read-only summary say so", () => {
  render(
    <>
      <ArticleTitleInput id="title" label="Title" placeholder="Title" value="Sourdough" readOnly />
      <ArticleTextArea label="Summary" placeholder="A line or two" value="A diary." readOnly />
    </>,
  );

  expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Title" }).readOnly).toBe(true);
  expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Summary" }).readOnly).toBe(true);
});

test("a field is a group named by its label, read in order: the label, what ends its line, the hint, the control, the error", () => {
  render(
    <ArticleField
      id="summary"
      label="Summary"
      end={<span>10 characters left</span>}
      hint="Shown with the title in timelines."
      error="The summary is too long."
    >
      <ArticleTextArea
        label="Summary"
        placeholder="A line or two"
        value="A diary."
        onChange={none}
        invalid
        describedBy="summary-error"
      />
    </ArticleField>,
  );
  const field = screen.getByRole("group", { name: "Summary" });
  const control = within(field).getByRole("textbox", { name: "Summary", description: "The summary is too long." });

  expect(control.getAttribute("aria-invalid")).toBe("true");
  expect(
    inOrder(
      within(field).getByText("10 characters left"),
      within(field).getByText("Shown with the title in timelines."),
      control,
      within(field).getByText("The summary is too long."),
    ),
  ).toBe(true);
});

test("a field with nothing wrong shows no error", () => {
  render(
    <ArticleField id="summary" label="Summary">
      <ArticleTextArea label="Summary" placeholder="A line or two" value="" onChange={none} />
    </ArticleField>,
  );

  const control = within(screen.getByRole("group", { name: "Summary" })).getByRole("textbox", { name: "Summary" });
  expect(control.getAttribute("aria-invalid")).toBeNull();
  expect(control.hasAttribute("aria-describedby")).toBe(false);
});

test("the summary is a field named by its label that tells the caller what is typed", async () => {
  const user = userEvent.setup();
  const typed: string[] = [];
  function Summary() {
    const [value, setValue] = useState("A diary");
    return (
      <ArticleTextArea
        label="Summary"
        placeholder="A line or two"
        value={value}
        onChange={(next) => {
          typed.push(next);
          setValue(next);
        }}
      />
    );
  }
  render(<Summary />);
  const summary = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Summary" });

  expect(summary.placeholder).toBe("A line or two");
  await user.type(summary, ".");

  expect(typed).toEqual(["A diary."]);
});

test("the cover's row has a field named for its description, and its picture is decoration", async () => {
  const user = userEvent.setup();
  const typed: string[] = [];
  function Cover() {
    const [description, setDescription] = useState("A loaf");
    return (
      <CoverRow
        id="cover"
        src="https://m.example/loaf.webp"
        description={description}
        onDescription={(next) => {
          typed.push(next);
          setDescription(next);
        }}
        descriptionLabel="Description"
        placeholder="What it shows"
      />
    );
  }
  render(<Cover />);
  const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Description" });

  expect(field.id).toBe("cover");
  expect(field.placeholder).toBe("What it shows");
  await user.type(field, "!");

  expect(typed).toEqual(["A loaf!"]);
  expect(screen.queryByRole("img")).toBeNull();
});

test("the cover's row says that it uploads, then that it failed, in a status that is there from the start", () => {
  const row = (status?: { text: string; busy: boolean }) => (
    <CoverRow
      id="cover"
      description=""
      onDescription={none}
      descriptionLabel="Description"
      placeholder="What it shows"
      status={status}
    />
  );
  const { rerender } = render(row());
  const status = screen.getByRole("status");
  expect(status.textContent).toBe("");

  rerender(row({ text: "Uploading…", busy: true }));
  expect(screen.getByRole("status") === status).toBe(true);
  expect(status.textContent).toBe("Uploading…");

  rerender(row({ text: "The upload did not finish.", busy: false }));
  expect(screen.getByRole("status") === status).toBe(true);
  expect(status.textContent).toBe("The upload did not finish.");
});

test("Add a cover opens the file chooser for one image, and gives the caller the file", async () => {
  const user = userEvent.setup();
  const picked: string[] = [];
  render(<CoverAdd label="Add a cover" onFile={(file) => picked.push(file.name)} />);
  const opened: HTMLInputElement[] = [];
  const click = spyOn(HTMLInputElement.prototype, "click").mockImplementation(function (this: HTMLInputElement) {
    if (this.type === "file") opened.push(this);
  });
  try {
    await user.click(screen.getByRole("button", { name: "Add a cover" }));
  } finally {
    click.mockRestore();
  }
  const [chooser] = opened;
  if (chooser === undefined) throw new Error("the press opened no file chooser");

  expect(chooser.accept).toBe("image/*");
  expect(chooser.multiple).toBe(false);
  await user.upload(chooser, new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], "loaf.png", { type: "image/png" }));

  expect(picked).toEqual(["loaf.png"]);
});

test("the cover's remove is a button in words", async () => {
  const user = userEvent.setup();
  let removed = 0;
  render(<CoverRemove label="Remove cover" onPress={() => removed++} />);

  await user.click(screen.getByRole("button", { name: "Remove cover" }));

  expect(removed).toBe(1);
});

/** The hashtags as a caller holds them, and what the field asked of it, in order. */
function Tags({ asked, initial = ["bread"], full }: { asked: string[]; initial?: string[]; full?: boolean }) {
  const [items, setItems] = useState(initial);
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        asked.push("submit");
      }}
    >
      <TagInput
        label="Hashtags"
        items={items}
        removeLabel={(tag) => `Remove #${tag}`}
        placeholder="Add a hashtag"
        full={full}
        onAdd={(tag) => {
          asked.push(`add ${tag}`);
          setItems((before) => [...before, tag]);
        }}
        onRemove={(tag) => {
          asked.push(`remove ${tag}`);
          setItems((before) => before.filter((item) => item !== tag));
        }}
      />
      <button type="submit">Publish</button>
    </form>
  );
}

const tagField = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Hashtags" });

test("each hashtag shows with its #, and its remove, named for it, asks for it", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Tags asked={asked} initial={["bread", "山"]} />);

  expect(screen.getByText("#bread")).not.toBeNull();
  expect(screen.getByText("#山")).not.toBeNull();
  expect(tagField().placeholder).toBe("Add a hashtag");
  await user.click(screen.getByRole("button", { name: "Remove #山" }));

  expect(asked).toEqual(["remove 山"]);
  expect(screen.queryByText("#山")).toBeNull();
});

test("what is typed becomes a hashtag, without its #, at Enter, a comma, a space or leaving the field", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Tags asked={asked} />);

  await user.type(tagField(), "rye{Enter}");
  await user.type(tagField(), "#oat,");
  await user.type(tagField(), "spelt ");
  await user.type(tagField(), "corn");
  await user.tab();

  // Enter in the field adds: it does not submit the form around it.
  expect(asked).toEqual(["add rye", "add oat", "add spelt", "add corn"]);
  expect(tagField().value).toBe("");
  for (const tag of ["#rye", "#oat", "#spelt", "#corn"]) expect(screen.getByText(tag)).not.toBeNull();
});

test("nothing typed adds nothing, and the Enter that ends an input method's composition does not add", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Tags asked={asked} />);

  await user.type(tagField(), "{Enter}, ");
  await user.type(tagField(), "やま");
  fireEvent.keyDown(tagField(), { key: "Enter", isComposing: true });

  expect(asked).toEqual([]);
  expect(tagField().value).toBe("やま");
});

test("Backspace in the empty field takes the last hashtag back into it to change", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Tags asked={asked} initial={["bread", "rye"]} />);

  await user.click(tagField());
  await user.keyboard("{Backspace}");

  expect(asked).toEqual(["remove rye"]);
  expect(tagField().value).toBe("rye");
  expect(screen.queryByText("#rye")).toBeNull();
  // In a field that holds something, Backspace is the field's own.
  await user.keyboard("{Backspace}");
  expect(asked).toEqual(["remove rye"]);
  expect(tagField().value).toBe("ry");
});

test("once full, the field takes no more", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Tags asked={asked} full />);

  expect(tagField().readOnly).toBe(true);
  await user.type(tagField(), "rye{Enter}");

  expect(asked).toEqual([]);
  expect(tagField().value).toBe("");
});

test("the limits read as their words, in order", () => {
  render(
    <ArticleLimits
      items={[
        { id: "bytes", text: "12,345 of 100,000 bytes", state: "ok" },
        { id: "images", text: "19 of 20 images", state: "near" },
        { id: "links", text: "101 of 100 links", state: "over" },
      ]}
    />,
  );

  expect(
    inOrder(
      screen.getByText("12,345 of 100,000 bytes"),
      screen.getByText("19 of 20 images"),
      screen.getByText("101 of 100 links"),
    ),
  ).toBe(true);
});

test("the preview is an article: its title a heading, its summary, its cover with its description, its body, in its language", () => {
  render(
    <ArticlePreview
      title="A sourdough starter"
      summary="A diary of a jar of flour."
      cover={{ src: "https://m.example/loaf.webp", alt: "A round loaf" }}
      lang="ja"
    >
      <p>Nothing, then bubbles.</p>
    </ArticlePreview>,
  );
  const preview = within(screen.getByRole("article"));

  expect(
    inOrder(
      preview.getByRole("heading", { level: 2, name: "A sourdough starter" }),
      preview.getByText("A diary of a jar of flour."),
      preview.getByRole("img", { name: "A round loaf" }),
      preview.getByText("Nothing, then bubbles."),
    ),
  ).toBe(true);
  expect(screen.getByRole("article").lang).toBe("ja");
  expect(screen.getByRole("article").getAttribute("aria-busy") ?? "false").toBe("false");
});

test("while it renders again the preview is busy, showing what it showed", () => {
  render(
    <ArticlePreview title="A sourdough starter" busy>
      <p>Nothing, then bubbles.</p>
    </ArticlePreview>,
  );

  expect(within(screen.getByRole("article", { busy: true })).getByText("Nothing, then bubbles.")).not.toBeNull();
});

test("the preview says that nothing is written yet, or shows the notice of a body that could not be rendered", () => {
  const { rerender } = render(<ArticlePreview title="A sourdough starter" empty="Nothing written yet." />);
  expect(within(screen.getByRole("article")).getByText("Nothing written yet.")).not.toBeNull();

  rerender(
    <ArticlePreview
      title="A sourdough starter"
      notice={
        <p role="alert">
          The preview could not be made. <button type="button">Try again</button>
        </p>
      }
    />,
  );
  const preview = within(screen.getByRole("article"));
  expect(preview.getByRole("alert")).not.toBeNull();
  expect(preview.getByRole("button", { name: "Try again" })).not.toBeNull();
});

test("the details are a region named by their heading", () => {
  render(
    <ArticleDetails id="details" heading="Details">
      <ArticleTextArea label="Summary" placeholder="A line or two" value="" onChange={none} />
    </ArticleDetails>,
  );
  const details = screen.getByRole("region", { name: "Details" });

  expect(within(details).getByRole("heading", { level: 2, name: "Details" })).not.toBeNull();
  expect(within(details).getByRole("textbox", { name: "Summary" })).not.toBeNull();
});
