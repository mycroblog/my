// SPDX-License-Identifier: AGPL-3.0-only

// The catalog, where the decider and an agent decide the tokens and the components with the notes of @my/devtools
// (ADR-0168): a page per layer (pages.ts), and at / an index of the pages and their sections. A link keeps the page's
// `?lang=` and opens in the top window, so that the overlay's device iframe follows it with its size and locale.

import { fontFamily, rootPx } from "@my/design-tokens";
import { useEffect, type ReactNode } from "react";
import { Buttons } from "./Buttons";
import { Contents } from "./Contents";
import { Motion } from "./Motion";
import { pageAt, pages, type Page, type SectionId } from "./pages";
import { PostStates } from "./PostStates";
import { Posts } from "./Posts";
import { Colors, TypeSpecimen } from "./Tokens";

const rem = (px: number) => `${px / rootPx}rem`;

const bodies: Record<SectionId, () => ReactNode> = {
  type: TypeSpecimen,
  color: Colors,
  button: Buttons,
  post: Posts,
  motion: Motion,
  content: Contents,
  "post-states": PostStates,
};

function Link({ to, children }: { to: string; children: ReactNode }) {
  const [path, hash = ""] = to.split("#");
  return (
    <a href={`${path ?? ""}${location.search}${hash && `#${hash}`}`} target="_top">
      {children}
    </a>
  );
}

function Index() {
  return (
    <>
      <h1>Catalog</h1>
      {pages.map((page: Page) => (
        <section key={page.path}>
          <h2>
            <Link to={page.path}>{page.title}</Link>
          </h2>
          <p>{page.about}</p>
          {page.sections.length > 0 && (
            <ul>
              {page.sections.map((s) => (
                <li key={s.id}>
                  <Link to={`${page.path}#${s.id}`}>{s.title}</Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      ))}
    </>
  );
}

function Sections({ page }: { page: Page }) {
  // The sections render after the browser looked for the URL's fragment: scroll to it once they are there.
  useEffect(() => {
    if (location.hash) document.getElementById(decodeURIComponent(location.hash.slice(1)))?.scrollIntoView();
  }, []);
  return (
    <>
      <p>
        <Link to="/">Catalog</Link>
      </p>
      <h1>{page.title}</h1>
      {page.sections.map(({ id, title }) => {
        const Body = bodies[id as SectionId];
        return (
          <section key={id} aria-labelledby={id}>
            <h2 id={id}>{title}</h2>
            <Body />
          </section>
        );
      })}
    </>
  );
}

/** The catalog page at a path: a layer's page, else the index. */
export function Catalog({ path }: { path: string }) {
  const page = pageAt(path);
  return (
    <main style={{ fontFamily: fontFamily.sans, margin: "0 auto", maxWidth: rem(960), padding: rem(24) }}>
      {page && page.sections.length > 0 ? <Sections page={page} /> : <Index />}
    </main>
  );
}
