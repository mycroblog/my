// SPDX-License-Identifier: AGPL-3.0-only

// The facts of a record and the figures of a server's state (#3258), as assistive technology gets them: the facts a
// description list named by its label, a kept value's Show read with its term and the value taking the focus once it
// comes; the figures a list, each its number and its label, a link where it leads somewhere. How they stand at a
// width is their stories' (Facts).

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { FactList, FigureTiles } from "./facts";

test("the facts are terms and their values, in a list named by its label", () => {
  render(
    <FactList
      label="Registration"
      items={[
        { term: "Account", value: "@river_tanaka" },
        { term: "Registered from", value: "203.0.113.42", code: true },
      ]}
    />,
  );
  const list = screen.getByLabelText("Registration");
  expect(list.tagName).toBe("DL");
  expect([...list.querySelectorAll("dt")].map((t) => t.textContent)).toEqual(["Account", "Registered from"]);
  expect([...list.querySelectorAll("dd")].map((d) => d.textContent)).toEqual(["@river_tanaka", "203.0.113.42"]);
});

test("a kept value is Show, read with its term, and the value takes the focus once it comes", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  function Facts() {
    const [email, setEmail] = useState<string | undefined>(undefined);
    return (
      <FactList
        label="Registration"
        items={[
          {
            term: "Email",
            ...(email === undefined ? {} : { value: email }),
            reveal: {
              onReveal: () => {
                asked.push("email");
                setEmail("river.tanaka@mail.example");
              },
            },
          },
        ]}
      />
    );
  }
  render(<Facts />);
  const show = screen.getByRole("button", { name: "Show" });
  expect(show.getAttribute("aria-describedby")).toBe(screen.getByText("Email").id);

  await user.click(show);

  expect(asked).toEqual(["email"]);
  expect(screen.queryByRole("button", { name: "Show" })).toBeNull();
  expect(document.activeElement?.textContent).toBe("river.tanaka@mail.example");
});

test("the figures are a list, each its number and its label, a link where it leads somewhere", () => {
  render(
    <FigureTiles
      label="This server"
      items={[
        { figure: "12", label: "Open reports", href: "/moderation/reports" },
        { figure: "1,204", label: "Jobs in the queue" },
      ]}
    />,
  );
  const list = screen.getByRole("list", { name: "This server" });
  const items = within(list).getAllByRole("listitem");
  expect(items.map((item) => item.textContent)).toEqual(["12Open reports", "1,204Jobs in the queue"]);
  expect(within(list).getByRole("link", { name: "12 Open reports" }).getAttribute("href")).toBe("/moderation/reports");
  expect(within(list).getAllByRole("link")).toHaveLength(1);
});
