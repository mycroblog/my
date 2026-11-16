// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { BackButton, ScreenTitle } from "./list";

/** The markup without classes and icons, to read the structure. */
const bare = (html: string) => html.replace(/ class="[^"]*"/g, "").replace(/<svg.*?<\/svg>/g, "");

const back = <BackButton onPress={() => undefined}>Back</BackButton>;

test("the back button is over the title, and the actions end the title's one row, after the name", () => {
  const html = bare(
    renderToStaticMarkup(
      <ScreenTitle back={back} actions={<button>Follow</button>}>
        #baking
      </ScreenTitle>,
    ),
  );
  expect(html).toMatch(
    />Back<\/button><\/p><div><h2><bdi>#baking<\/bdi><\/h2><div><button>Follow<\/button><\/div><\/div>/,
  );
});

test("a long name may break before each capital inside it, and not inside a word of small letters", () => {
  const html = renderToStaticMarkup(<ScreenTitle>#TheLongestHashtagAboutSlowMornings</ScreenTitle>);
  expect(html).toContain("<bdi>#The<wbr/>Longest<wbr/>Hashtag<wbr/>About<wbr/>Slow<wbr/>Mornings</bdi>");
  expect(renderToStaticMarkup(<ScreenTitle>#slowmornings</ScreenTitle>)).toContain("<bdi>#slowmornings</bdi>");
});

test("the count follows the name in the heading, muted, apart by a space, and none shows without one", () => {
  const html = renderToStaticMarkup(
    <ScreenTitle back={back} glyph="list" count="3 members">
      Close friends
    </ScreenTitle>,
  );
  expect(html).toMatch(/<bdi>Close friends<\/bdi> <span class="[^"]*text-muted-text[^"]*">3 members<\/span><\/h2>/);
  expect(bare(renderToStaticMarkup(<ScreenTitle>#baking</ScreenTitle>))).toMatch(/<h2><bdi>#baking<\/bdi><\/h2>/);
});
