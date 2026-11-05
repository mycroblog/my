// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";
import { ariaSnapshot } from "../test/aria-snapshot";
import { ExternalLinkContext } from "./ExternalLink";
import type { PostContentNode } from "./PostContent";
import {
  PostContent,
  PostContentNodes,
  sanitizedHtmlNodes,
  sanitizedHtmlText,
  serverSanitizedHtml,
} from "./PostContent";

/**
 * The name of a link followed by the hidden "(opens in a new tab)". jsdom lays nothing out, so the name it computes
 * has no space between the two; a browser's has one, as the hidden text is a block (`sr-only` is absolute).
 */
function newTab(name: string): RegExp {
  return new RegExp(`^${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")} ?\\(opens in a new tab\\)$`);
}

/** The app's settings of the links: its own pages are the hashtag pages of example.com. */
function withLinks(children: ReactNode) {
  return (
    <ExternalLinkContext
      value={{
        newTabLabel: "(opens in a new tab)",
        isAppLink: (url) => url.host === "example.com" && url.pathname.startsWith("/tags/"),
      }}
    >
      {children}
    </ExternalLinkContext>
  );
}

describe("PostContent", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders the sanitized HTML with the post's own direction and language", () => {
    const { container } = render(
      <PostContent html={serverSanitizedHtml('<p>مرحبا <a href="https://example.com/">#tag</a></p>')} lang="ar" />,
    );
    expect(ariaSnapshot(container)).toBe(
      [`- generic [lang=ar] [dir=auto]:`, `  - text: "مرحبا"`, `  - link "#tag"`].join("\n"),
    );
  });

  it("accepts only HTML marked as server-sanitized", () => {
    // @ts-expect-error -- a plain string is not SanitizedHtml; this line is the type test.
    render(<PostContent html="<p>unchecked</p>" />);
    expect(screen.getByText("unchecked")).toBeInTheDocument();
  });

  it("opens the links to the app's own pages in the same tab, and every other link in a new tab", () => {
    // As the server renders a post: every link has the same rel and target.
    const rel = 'rel="nofollow noopener noreferrer" target="_blank"';
    const html = serverSanitizedHtml(
      `<p><a href="https://example.com/tags/cats" class="mention hashtag" ${rel}>#<span>cats</span></a> <a href="https://news.example/a" ${rel}>news.example/a</a> <a href="https://news.example/b">b</a></p>`,
    );
    const { rerender } = render(<PostContent html={html} />, { wrapper: ({ children }) => withLinks(children) });

    const tag = screen.getByRole("link", { name: "#cats" });
    expect(tag).not.toHaveAttribute("target");
    expect(tag).toHaveAttribute("rel", "nofollow noopener noreferrer");
    const news = screen.getByRole("link", { name: newTab("news.example/a") });
    expect(news).toHaveAttribute("target", "_blank");
    expect(news).toHaveAttribute("rel", "noopener noreferrer nofollow");
    const plain = screen.getByRole("link", { name: newTab("b") });
    expect(plain).toHaveAttribute("target", "_blank");
    expect(plain).toHaveAttribute("rel", "noopener noreferrer");

    // Rendered again, the links get the text once.
    rerender(<PostContent html={html} className="text-sm" />);
    expect(screen.getByRole("link", { name: newTab("news.example/a") })).toBeInTheDocument();
    rerender(<PostContent html={serverSanitizedHtml(`<p><a href="https://news.example/c" ${rel}>c</a></p>`)} />);
    expect(screen.getByRole("link", { name: newTab("c") })).toHaveAttribute("target", "_blank");
  });

  it("hands the links to the app's own pages followed in the same tab to onLinkClick", async () => {
    const user = userEvent.setup();
    const followed: string[] = [];
    const onLinkClick = vi.fn((event: MouseEvent, link: HTMLAnchorElement) => {
      followed.push(link.href);
      // jsdom does not navigate; the handler keeps the browser from it as the app does.
      event.preventDefault();
    });
    const html = serverSanitizedHtml(
      '<p><a href="https://example.com/tags/cats" target="_blank"><span>#</span>cats</a> <a href="https://example.com/x" target="_blank">x</a> <a href="https://example.com/tags/dogs" download>y</a></p>',
    );
    render(<PostContent html={html} onLinkClick={onLinkClick} />, { wrapper: ({ children }) => withLinks(children) });
    // The links left to the browser would make jsdom log "Not implemented: navigation"; stop them once React has seen
    // them, after onLinkClick would have been called.
    const stayOnThePage = (event: MouseEvent) => {
      event.preventDefault();
    };
    document.addEventListener("click", stayOnThePage);
    onTestFinished(() => {
      document.removeEventListener("click", stayOnThePage);
    });

    await user.click(screen.getByText("cats"));
    await user.click(screen.getByText("#"));
    await user.keyboard("{Enter}");
    await user.click(screen.getByRole("link", { name: newTab("x") }));
    await user.click(screen.getByRole("link", { name: "y" }));
    await user.keyboard("{Control>}");
    await user.click(screen.getByRole("link", { name: "#cats" }));
    await user.keyboard("{/Control}");

    expect(followed).toEqual([
      "https://example.com/tags/cats",
      "https://example.com/tags/cats",
      "https://example.com/tags/cats",
    ]);
    expect(onLinkClick.mock.calls.every(([event]) => event.defaultPrevented)).toBe(true);
  });

  it("passes the HTML through a Trusted Types policy where the browser has Trusted Types", () => {
    // A stand-in for the browser's factory: its TrustedHTML is an object that innerHTML turns into its string.
    const createHTML = vi.fn((html: string) => ({ toString: () => html }));
    const createPolicy = vi.fn((_name: string, rules: { createHTML: (html: string) => string }) => ({
      createHTML: (html: string) => createHTML(rules.createHTML(html)),
    }));
    vi.stubGlobal("trustedTypes", { createPolicy });

    render(<PostContent html={serverSanitizedHtml("<p>trusted</p>")} />);
    render(<PostContent html={serverSanitizedHtml("<p>again</p>")} />);

    expect(screen.getByText("trusted")).toBeInTheDocument();
    expect(screen.getByText("again")).toBeInTheDocument();
    expect(createPolicy).toHaveBeenCalledOnce();
    expect(createHTML).toHaveBeenCalledWith("<p>trusted</p>");
  });
});

describe("sanitizedHtmlNodes", () => {
  it("reads the structure of the body as data, with its text as text", () => {
    expect(
      sanitizedHtmlNodes(
        serverSanitizedHtml(
          '<p lang="fr">one &lt;b&gt; <a class="mention" href="https://example.com/">two</a><br></p>',
        ),
      ),
    ).toEqual([
      {
        tag: "p",
        attributes: { lang: "fr" },
        children: [
          "one <b> ",
          { tag: "a", attributes: { class: "mention", href: "https://example.com/" }, children: ["two"] },
          { tag: "br", attributes: {}, children: [] },
        ],
      },
    ]);
  });
});

describe("PostContentNodes", () => {
  const element = (tag: string, children: PostContentNode[], attributes: Record<string, string> = {}) => ({
    tag,
    attributes,
    children,
  });

  it("shows markup in text as text", () => {
    render(<PostContentNodes nodes={[element("p", ['<img src=x onerror="alert(1)"><b>bold</b>'])]} lang="en" />);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.queryByText("bold", { selector: "b" })).not.toBeInTheDocument();
    const paragraph = screen.getByText('<img src=x onerror="alert(1)"><b>bold</b>', { selector: "p" });
    expect(paragraph).toBeInTheDocument();
    expect(
      screen.getByText(
        (_, node) =>
          node?.getAttribute("dir") === "auto" &&
          node.getAttribute("lang") === "en" &&
          node.textContent === paragraph.textContent,
      ),
    ).toBeInTheDocument();
  });

  it("draws only the elements of the allowlist", () => {
    render(<PostContentNodes nodes={[element("p", [element("iframe", ["inside"])])]} />);
    expect(screen.queryByText("inside", { selector: "iframe" })).not.toBeInTheDocument();
    expect(screen.getByText("inside", { selector: "p" })).toBeInTheDocument();
  });

  it("opens a link to another site in a new tab, with the words that say so, and one of the app's own pages in place", async () => {
    const onLinkClick = vi.fn((event: MouseEvent) => {
      event.preventDefault();
    });
    render(
      withLinks(
        <PostContentNodes
          onLinkClick={onLinkClick}
          nodes={[
            element("p", [
              element("a", ["outside"], { href: "https://other.example/", rel: "nofollow" }),
              element("a", ["#inside"], { href: "https://example.com/tags/inside", target: "_blank" }),
            ]),
          ]}
        />,
      ),
    );
    const outside = screen.getByRole("link", { name: newTab("outside") });
    expect(outside).toHaveAttribute("target", "_blank");
    expect(outside).toHaveAttribute("rel", "noopener noreferrer nofollow");
    const inside = screen.getByRole("link", { name: "#inside" });
    expect(inside).not.toHaveAttribute("target");

    await userEvent.click(inside);
    expect(onLinkClick).toHaveBeenCalledOnce();
  });

  it("draws custom emoji in text and leaves them in code as the author typed them", () => {
    render(
      <PostContentNodes
        emojis={[{ shortcode: "blob", url: "https://example.com/a.png", static_url: "https://example.com/s.png" }]}
        nodes={[element("p", ["hi :blob:", element("code", [":blob:"])])]}
      />,
    );
    expect(screen.getByAltText(":blob:")).toBeInTheDocument();
    expect(screen.getByText(":blob:").tagName).toBe("CODE");
  });
});

describe("sanitizedHtmlText", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("returns the text without markup, with paragraphs and line breaks as line breaks", () => {
    expect(
      sanitizedHtmlText(serverSanitizedHtml('<p>one<br>two</p><p> <a href="https://example.com/">three</a> </p>')),
    ).toBe("one\ntwo\nthree");
    expect(sanitizedHtmlText(serverSanitizedHtml("plain &amp; text"))).toBe("plain & text");
    expect(sanitizedHtmlText(serverSanitizedHtml(""))).toBe("");
  });

  it("parses the HTML as a TrustedHTML of PostContent's policy where the browser has Trusted Types", () => {
    // A stand-in for the browser's factory: its TrustedHTML is an object that DOMParser turns into its string.
    const createHTML = vi.fn((html: string) => ({ toString: () => html }));
    const createPolicy = vi.fn((_name: string, rules: { createHTML: (html: string) => string }) => ({
      createHTML: (html: string) => createHTML(rules.createHTML(html)),
    }));
    vi.stubGlobal("trustedTypes", { createPolicy });
    const parseFromString = vi.spyOn(DOMParser.prototype, "parseFromString");

    expect(sanitizedHtmlText(serverSanitizedHtml("<p>trusted</p>"))).toBe("trusted");
    render(<PostContent html={serverSanitizedHtml("<p>shown</p>")} />);

    expect(screen.getByText("shown")).toBeInTheDocument();
    expect(createPolicy).toHaveBeenCalledOnce();
    expect(createPolicy).toHaveBeenCalledWith("my-server-sanitized-html", expect.anything());
    expect(parseFromString.mock.calls[0]?.[0]).toBe(createHTML.mock.results[0]?.value);
  });
});
