// SPDX-License-Identifier: AGPL-3.0-only
import { afterEach, describe, expect, test } from "vitest";
import { measureScreen } from "./screenMeasure";

function show(html: string): void {
  document.body.innerHTML = html;
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("measureScreen", () => {
  test("counts the controls, helper texts and words of main, and leaves the shell out", () => {
    show(`
      <nav><a href="/home">Home</a><button>Post</button></nav>
      <main>
        <h1>Rules</h1>
        <p data-helper="hint">Shown on the about page.</p>
        <a href="/admin/rules/new">Add a rule</a>
        <label>Name <input /></label><span slot="description">Up to 30 letters.</span>
        <input type="hidden" /><button aria-hidden="true"></button>
      </main>`);
    expect(measureScreen()).toEqual({ controls: 2, helpers: 2, words: 14, regions: [] });
  });

  test("counts a radio group as one control and a control inside another once", () => {
    show(`
      <main>
        <div role="radiogroup"><input type="radio" /><input type="radio" /><input type="radio" /></div>
        <a href="/x"><button>Open</button></a>
      </main>`);
    expect(measureScreen().controls).toBe(2);
  });

  test("counts a list or a table as its largest row", () => {
    show(`
      <main>
        <table><tbody>
          <tr><td><a href="/1">Be kind</a></td> <td><button>Up</button> <button>Down</button></td></tr>
          <tr><td><a href="/2">No spam at all</a></td> <td><button>Up</button></td></tr>
        </tbody></table>
        <ul><li><p data-helper="hint">one</p></li> <li><p data-helper="hint">two</p></li></ul>
      </main>`);
    expect(measureScreen()).toEqual({ controls: 3, helpers: 1, words: 6, regions: [] });
  });

  test("lists the regions under an h2 in the page's order with the controls of each", () => {
    show(`
      <main>
        <button>Outside</button>
        <section><h2>Your  passkeys</h2><button>Add</button><button>Rename</button></section>
        <section><h2>Delete</h2><div data-helper="callout">Careful. <button>Delete</button></div></section>
      </main>`);
    const measure = measureScreen();
    expect(measure.regions).toEqual([
      { title: "Your passkeys", controls: 2 },
      { title: "Delete", controls: 1 },
    ]);
    expect(measure.controls).toBe(4);
  });

  test("adds a dialog open outside main", () => {
    show(`<main><button>Delete</button></main><div role="dialog"><h2>Sure?</h2><button>Yes</button></div>`);
    expect(measureScreen()).toMatchObject({ controls: 2, regions: [{ title: "Sure?", controls: 1 }] });
  });
});
