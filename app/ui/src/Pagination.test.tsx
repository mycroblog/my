// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Pagination, pageEntries } from "./Pagination";

const labels = { label: "Pages", previousLabel: "Previous", nextLabel: "Next" };

describe("pageEntries", () => {
  it("lists every page when there are few", () => {
    expect(pageEntries(2, 7, 1)).toEqual([1, 2, 3, 4, 5, 6, 7]);
  });
  it("folds the far side of the current page", () => {
    expect(pageEntries(1, 20, 1)).toEqual([1, 2, 3, 4, 5, "end-ellipsis", 20]);
    expect(pageEntries(10, 20, 1)).toEqual([1, "start-ellipsis", 9, 10, 11, "end-ellipsis", 20]);
    expect(pageEntries(20, 20, 1)).toEqual([1, "start-ellipsis", 16, 17, 18, 19, 20]);
  });
});

describe("Pagination", () => {
  it("marks the current page and names the landmark", () => {
    render(<Pagination {...labels} page={2} pageCount={3} onPageChange={() => {}} />);
    expect(screen.getByRole("navigation", { name: "Pages" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "2" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "3" })).not.toHaveAttribute("aria-current");
  });

  it("disables the ends that lead nowhere", () => {
    render(<Pagination {...labels} page={1} pageCount={3} onPageChange={() => {}} />);
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).toBeEnabled();
  });

  it("reports the page a button leads to", async () => {
    const onPageChange = vi.fn();
    render(<Pagination {...labels} page={2} pageCount={5} onPageChange={onPageChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    await userEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(onPageChange.mock.calls).toEqual([[3], [1]]);
  });

  it("renders links with getHref", () => {
    render(
      <Pagination {...labels} page={2} pageCount={3} getHref={(p) => `?page=${p}`} pageLabel={(p) => `Page ${p}`} />,
    );
    expect(screen.getByRole("link", { name: "Page 2" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Page 3" })).toHaveAttribute("href", "?page=3");
    expect(screen.getByRole("link", { name: "Next" })).toHaveAttribute("href", "?page=3");
  });

  it("shows only the ends for a cursor list", () => {
    render(<Pagination {...labels} previous={null} next={{ href: "?after=abc" }} />);
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("link", { name: "Next" })).toHaveAttribute("href", "?after=abc");
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });
});
