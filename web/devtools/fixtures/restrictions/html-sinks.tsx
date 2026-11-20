// SPDX-License-Identifier: AGPL-3.0-only
// restrict/html-sinks: no string reaches the browser as markup, which the shell's Trusted Types refuse (#3295).
// web/devtools/src/restrictions.test.js lints it; the lint of the package leaves it out.

const markup = "<b>bold</b>";

export function sinks(element: HTMLElement, frame: HTMLIFrameElement) {
  element.innerHTML = markup; // error: html .innerHTML =
  element.innerHTML += markup; // error: html .innerHTML =
  element.outerHTML = markup; // error: html .outerHTML =
  element["innerHTML"] = markup; // error: html .innerHTML =
  frame.srcdoc = markup; // error: html .srcdoc =
  frame.setAttribute("srcdoc", markup); // error: html setAttribute("srcdoc")
  element.insertAdjacentHTML("beforeend", markup); // error: html .insertAdjacentHTML()
  const parsed = new DOMParser().parseFromString(markup, "text/html"); // error: html DOMParser
  const Parser = window.DOMParser; // error: html .DOMParser
  return [parsed, Parser];
}

const props = { dangerouslySetInnerHTML: { __html: markup } }; // error: html dangerouslySetInnerHTML

export const Sinks = () => (
  <>
    <div dangerouslySetInnerHTML={{ __html: markup }} /> {/* error: html dangerouslySetInnerHTML */}
    <iframe title="frame" srcDoc={markup} /> {/* error: html srcDoc */}
    <div {...props} />
  </>
);

// What hands the browser no markup: a read of the markup, text, elements, and a name of the file's own.
export function allowed(element: HTMLElement, DOMParser: { parse: (s: string) => string }) {
  const read = element.innerHTML + element.outerHTML;
  element.textContent = markup;
  element.replaceChildren(document.createElement("b"));
  element.setAttribute("title", markup);
  return [read, DOMParser.parse(markup)];
}
