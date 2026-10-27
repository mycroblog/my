// SPDX-License-Identifier: AGPL-3.0-only
// The one audited component that renders HTML (web-client/README.md A8), and the one function that parses it
// (`sanitizedHtmlText`). Lint allows dangerouslySetInnerHTML and the other HTML sinks in this file only
// (app/.oxlintrc.json). The server sanitizes every post body to its allowlist
// before it reaches the client; this component trusts that and adds nothing of its own. The app's
// Content-Security-Policy requires Trusted Types (`require-trusted-types-for 'script'`, ADR-0019 §9), under
// which the browser refuses a plain string as innerHTML or to DOMParser, so the HTML goes in as a TrustedHTML
// made by the one policy below, the only place that turns a string into TrustedHTML. The shell's CSP allows this
// policy's name once (`trusted-types my-server-sanitized-html …`), so no other script can make a policy
// of its own, and this module makes it once.
//
// The policy passes the HTML through unchanged. That is safe only because every HTML it receives comes
// out of the server's sanitizer (the allowlist in the Go server); HTML from anywhere else, such as the
// remote posts that federation brings (M3), must go through the same sanitizer on the server first.
import { type ComponentPropsWithRef, type RefCallback, use } from "react";
import { mergeRefs } from "react-aria";
import { tv } from "tailwind-variants";
import { type CustomEmojiImage, insertCustomEmojis, noCustomEmojis } from "./CustomEmoji";
import { ExternalLinkContext, type ExternalLinkSettings, externalRel } from "./ExternalLink";
import { useAnimationsPlay } from "./Picture";

declare const sanitizedHtmlBrand: unique symbol;

/**
 * HTML that the server has already sanitized, such as the rendered body of a post in an API response.
 * A plain `string` is not assignable to it, so HTML from anywhere else (user input, a URL, a template)
 * cannot reach `PostContent` by accident: it has to pass through `serverSanitizedHtml`.
 */
export type SanitizedHtml = string & { readonly [sanitizedHtmlBrand]: true };

/**
 * Marks a string as server-sanitized HTML. Call it only on a field the server documents as sanitized
 * HTML, at the place the API response is read; never on text the client built or received from elsewhere.
 */
export function serverSanitizedHtml(html: string): SanitizedHtml {
  return html as SanitizedHtml;
}

/** The part of the Trusted Types API (https://developer.mozilla.org/docs/Web/API/Trusted_Types_API) used here. */
type TrustedTypePolicyFactory = {
  createPolicy: (name: string, rules: { createHTML: (html: string) => string }) => TrustedTypePolicy;
};
type TrustedTypePolicy = { createHTML: (html: string) => unknown };

/** The policy of each Trusted Types factory, made on first use. */
const policies = new WeakMap<TrustedTypePolicyFactory, TrustedTypePolicy>();

/**
 * The HTML as a TrustedHTML where the browser has Trusted Types, and as it is elsewhere. The policy
 * passes the HTML through unchanged: only `SanitizedHtml`, which the server sanitized, reaches it.
 */
function trustedHtml(html: SanitizedHtml): string {
  const factory = (globalThis as { trustedTypes?: TrustedTypePolicyFactory }).trustedTypes;
  if (factory === undefined) return html;
  let policy = policies.get(factory);
  if (policy === undefined) {
    policy = factory.createPolicy("my-server-sanitized-html", { createHTML: (input) => input });
    policies.set(factory, policy);
  }
  // React assigns the value of __html to innerHTML as it is, and innerHTML accepts the TrustedHTML in
  // place of the string its DOM type names.
  return policy.createHTML(html) as string;
}

/** DOMParser as it takes the TrustedHTML of `trustedHtml`, which its DOM type names a string. */
type HtmlParser = { parseFromString: (html: string, type: "text/html") => Document };

/**
 * The text of server-sanitized HTML without its markup, with its paragraphs and line breaks kept as line
 * breaks: for a place that quotes a post as plain text, such as a link to it. The HTML goes to DOMParser
 * through the same policy as `PostContent`'s, so it parses under Trusted Types too, into an inert
 * document in which nothing loads or runs.
 */
export function sanitizedHtmlText(html: SanitizedHtml): string {
  const parser: HtmlParser = new DOMParser();
  const doc = parser.parseFromString(trustedHtml(html), "text/html");
  for (const br of doc.body.querySelectorAll("br")) br.replaceWith("\n");
  const blocks = [...doc.body.children].map((element) => element.textContent.trim());
  return (blocks.length > 0 ? blocks.join("\n") : doc.body.textContent).trim();
}

const postContent = tv({
  // Long URLs and words wrap anywhere, so no post scrolls sideways at 320 px (web-client/README.md D3).
  base: [
    "text-base leading-[1.85] wrap-anywhere text-fg",
    "[&_a]:text-accent [&_a]:underline [&_a]:underline-offset-[3px] [&_a:hover]:text-accent-hover",
    // The server shortens the text of a URL link as Mastodon does: the scheme (and "www.") and what follows the
    // first characters are `invisible` spans, and the shown part is `ellipsis` when the rest is cut. The hidden parts
    // take no room but stay visible to selection, so copying the link copies the whole URL (Tailwind's own
    // `invisible` utility would only hide them and keep their width).
    "[&_.invisible]:visible [&_.invisible]:absolute [&_.invisible]:inline-block [&_.invisible]:size-0 [&_.invisible]:overflow-hidden [&_.invisible]:text-[0px] [&_.invisible]:leading-0",
    "[&_.ellipsis]:after:content-['…']",
    "[&_blockquote]:rounded-md [&_blockquote]:bg-surface-sunken [&_blockquote]:px-3 [&_blockquote]:py-2 [&_blockquote]:text-fg-muted",
    "[&_code]:rounded-sm [&_code]:bg-surface-sunken [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.9em]",
    "[&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-surface-sunken [&_pre]:p-3 [&_pre]:font-mono",
    "[&_pre_code]:bg-transparent [&_pre_code]:p-0",
    "[&_ol]:list-decimal [&_ol]:ps-6 [&_ul]:list-disc [&_ul]:ps-6",
    "[&>*+*]:mt-2",
  ],
  variants: {
    size: { sm: "text-sm", base: "", lg: "text-lg" },
    framed: { true: "rounded-md border border-border p-3" },
    lineClamp: { 2: "line-clamp-2", 3: "line-clamp-3" },
  },
});

export type PostContentProps = Omit<ComponentPropsWithRef<"div">, "children" | "dangerouslySetInnerHTML" | "dir"> & {
  /** The post body as the server rendered and sanitized it. */
  html: SanitizedHtml;
  /** `lg` for the post a page is about, in larger text; `sm` for an excerpt in a form; `base` by default. */
  size?: "sm" | "base" | "lg";
  /** Draws a border and padding around the body, for a preview of it in a form or a list of settings. */
  framed?: boolean;
  /** Cuts the body after this many lines, for an excerpt or a preview. */
  lineClamp?: 2 | 3;
  /** The language of the post, when the server knows it (a BCP 47 tag). */
  lang?: string | undefined;
  /**
   * Called when a link in the body to one of the app's own pages (`isAppLink` of `ExternalLinkContext`) is
   * followed in the same tab: a primary click, or Enter on the focused link, without a modifier key, on a link
   * that has no `download`; every other link opens in a new tab. Calling
   * `event.preventDefault()` keeps the browser from following it, so the app can open the link itself.
   * Opening a link in a new tab or window stays the browser's.
   */
  onLinkClick?: ((event: MouseEvent, link: HTMLAnchorElement) => void) | undefined;
  /**
   * The custom emoji the body uses (the API's `emojis`): each `:shortcode:` of one of them in the body's
   * text is drawn as its image, made with the DOM API rather than HTML (ADR-0019 §2, ADR-0051 §4).
   */
  emojis?: readonly CustomEmojiImage[] | undefined;
  /**
   * Whether animated emoji may move when the user allows motion; false shows their still images. The default is the
   * app's `AutoplayContext`, or the post it is in while that plays (`AnimationsPlayContext`).
   */
  emojiMotion?: boolean | undefined;
};

/**
 * Listens for the clicks on the links of the body. The links are in HTML that React does not render,
 * so the listener goes on the element itself; React removes it with the element or for a new handler.
 */
function linkClickListener(onLinkClick: PostContentProps["onLinkClick"]): RefCallback<HTMLDivElement> | undefined {
  if (onLinkClick === undefined) return undefined;
  return (element) => {
    if (element === null) return;
    const listener = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const link = event.target instanceof Element ? event.target.closest("a[href]") : null;
      if (!(link instanceof HTMLAnchorElement) || !element.contains(link) || link.target !== "" || link.hasAttribute("download")) return;
      onLinkClick(event, link);
    };
    element.addEventListener("click", listener);
    return () => {
      element.removeEventListener("click", listener);
    };
  };
}

/** The attribute that marks the "opens in a new tab" text of a link of the body, so that it comes out again. */
const newTabMark = "data-new-tab";

/**
 * Sets how each link of the body opens (web-client/README.md Y12). The server writes `target="_blank"` on every link of
 * the HTML it renders; a link to one of the app's own pages (`isAppLink`) loses it, so that it opens in the same
 * tab and `onLinkClick` can take it, and every other link keeps it, with `noopener noreferrer` added to the
 * server's `rel` and the "opens in a new tab" text after its content. The links are in HTML that React does not
 * render, so they change after React set it, and change back before React replaces it.
 */
function linkTargetSetter(html: SanitizedHtml, { newTabLabel, isAppLink }: ExternalLinkSettings): RefCallback<HTMLDivElement> | undefined {
  if (html === "") return undefined;
  return (element) => {
    if (element === null) return;
    const restores: (() => void)[] = [];
    for (const link of element.querySelectorAll<HTMLAnchorElement>("a[href]")) {
      let url: URL;
      try {
        url = new URL(link.href);
      } catch {
        continue;
      }
      const target = link.getAttribute("target");
      const rel = link.getAttribute("rel");
      if (isAppLink(url)) {
        link.removeAttribute("target");
      } else {
        link.setAttribute("target", "_blank");
        link.setAttribute("rel", externalRel(rel ?? undefined));
        if (newTabLabel !== undefined) {
          const label = link.ownerDocument.createElement("span");
          label.className = "sr-only";
          label.setAttribute(newTabMark, "");
          label.textContent = ` ${newTabLabel}`;
          link.append(label);
        }
      }
      restores.push(() => {
        for (const label of link.querySelectorAll(`:scope > [${newTabMark}]`)) label.remove();
        if (target === null) link.removeAttribute("target");
        else link.setAttribute("target", target);
        if (rel === null) link.removeAttribute("rel");
        else link.setAttribute("rel", rel);
      });
    }
    return () => {
      for (const restore of restores) restore();
    };
  };
}

/**
 * Draws the custom emoji in the body. The body is HTML that React does not render, so the emoji go in
 * after React set it, and come out again before React replaces it: the callback is made anew whenever
 * the body or its emoji change, and React calls the cleanup of the previous one first.
 */
function emojiDrawer(html: SanitizedHtml, emojis: readonly CustomEmojiImage[], allowMotion: boolean): RefCallback<HTMLDivElement> | undefined {
  if (emojis.length === 0 || html === "") return undefined;
  return (element) => {
    if (element === null) return;
    return insertCustomEmojis(element, emojis, allowMotion);
  };
}

/**
 * The body of a post: server-sanitized HTML in a block whose direction follows its own text (`dir="auto"`)
 * and whose `lang` is the post's language when known (web-client/README.md Y10). Give it an `id` to describe a
 * PostCard by it, `onLinkClick` to open the links of the body itself (the app opens the links to its
 * own pages through its router), and `emojis` to draw its custom emoji. The links to any other page open in a
 * new tab (`ExternalLinkContext`).
 */
export function PostContent({ html, framed, lang, onLinkClick, emojis = noCustomEmojis, emojiMotion: motion, size, lineClamp, className, ref, ...props }: PostContentProps) {
  const plays = useAnimationsPlay();
  const links = use(ExternalLinkContext);
  const emojiMotion = motion ?? plays;
  return (
    <div
      {...props}
      ref={mergeRefs(ref, linkTargetSetter(html, links), linkClickListener(onLinkClick), emojiDrawer(html, emojis, emojiMotion))}
      dir="auto"
      lang={lang}
      className={postContent({ size, framed, lineClamp, className })}
      dangerouslySetInnerHTML={{ __html: trustedHtml(html) }}
    />
  );
}
