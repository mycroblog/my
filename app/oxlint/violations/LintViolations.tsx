// SPDX-License-Identifier: AGPL-3.0-only
// Violations of the 強制 rows of docs/architecture/web-client/README.md that lint checks, as a feature's component:
// violations.test.js copies this file to web/src/features/timeline/components/, lints it, and requires
// exactly the reports that the `expect:` comments name on each line. React Compiler stops at the first
// problem of a function, so each React rule gets a function of its own.
import { createContext, forwardRef, useEffect, useMemo, useState, type FC } from "react"; // expect: eslint(no-restricted-imports) R5
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useVirtualizer } from "@tanstack/react-virtual";
import { PiStar } from "react-icons/pi"; // expect: eslint(no-restricted-imports) G10
import { tv } from "tailwind-variants"; // expect: restrict(classes) A4
import { useComposeForm } from "../../compose/hooks/useComposeForm"; // expect: boundaries(dependencies) A3
import { Route as IndexRoute } from "../../../routes/index"; // expect: boundaries(dependencies) A1
import { client } from "../../../lib/api/client"; // expect: eslint(no-restricted-imports) S1
import { createClient } from "@mycroblog/mykit"; // expect: eslint(no-restricted-imports) S1
import type { components } from "@mycroblog/mykit";
import { Button, Heading, MoreIcon, Stack } from "@my/ui";

let renders = 0;
const CountContext = createContext(0);
declare const api: { query: { useQuery: (method: string, path: string) => number } };

// R1
export function Clock() {
  const now = Date.now(); // expect: react(purity)
  return <p>{now}</p>;
}
export function Counter() {
  renders = renders + 1; // expect: react(globals)
  return <p>{renders}</p>;
}

// R2
export function Sorted({ items }: { items: string[] }) {
  items[0] = ""; // expect: react(immutability)
  return <p>{items.length}</p>;
}

// R3
export function useMaybe(on: boolean) {
  if (on) useEffect(() => undefined); // expect: react-hooks(rules-of-hooks)
}

// R4
export function Rows() {
  "use no memo"; // expect: restrict(syntax)
  return null;
}
export function useVirtual(count: number) {
  // app/web has no types of TanStack Virtual, so tsgolint sees the virtualizer as any.
  const virtualizer = useVirtualizer({ count, getScrollElement: () => null, estimateSize: () => 10 }); // expect: react(incompatible-library) typescript(no-unsafe-assignment) typescript(no-unsafe-call)
  return virtualizer.getTotalSize(); // expect: typescript(no-unsafe-call) typescript(no-unsafe-member-access) typescript(no-unsafe-return)
}
export function useInstance() {
  return api.query.useQuery("get", "/instance"); // expect: restrict(syntax)
}

// R5
export function useDoubled(n: number) {
  const doubled = useMemo(async () => n * 2, [n]); // expect: react(use-memo) typescript(require-await)
  return doubled;
}

// R6
export function Derived({ first }: { first: string }) {
  const [full, setFull] = useState("");
  useEffect(() => {
    setFull(first); // expect: react(set-state-in-effect) react(no-deriving-state-in-effects)
  }, [first]);
  return <p>{full}</p>;
}

// R9
export function Logger({ a }: { a: number }) {
  useEffect(() => {
    console.info(a); // expect: react-hooks(exhaustive-deps)
  }, []);
  return null;
}
export function Logged({ a }: { a: number }) {
  useEffect(() => {
    console.info(a);
    // eslint-disable-next-line react/exhaustive-deps -- R9 forbids this. expect: @eslint-community/eslint-comments(no-restricted-disable)
  }, []);
  useEffect(() => {
    console.info(a);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- R9 forbids this. expect: @eslint-community/eslint-comments(no-restricted-disable)
  }, []);
  return null;
}

// R11
export function Outer() {
  function Inner() {
    return null;
  }
  return <Inner />; // expect: react(static-components)
}

// R12
export function List({ items }: { items: string[] }) {
  return (
    <ul>
      {items.map((item, i) => (
        <li key={i}>{item}</li> // expect: @eslint-react(no-array-index-key)
      ))}
      {items.map((item) => (
        <li>{item}</li> // expect: @eslint-react(no-missing-key)
      ))}
    </ul>
  );
}

// R13
export const Field = forwardRef<HTMLInputElement>(function Field(_, ref) { // expect: @eslint-react(no-forward-ref)
  return <input ref={ref} />;
});
export function Provided() {
  return <CountContext.Provider value={1} />; // expect: @eslint-react(no-context-provider)
}
Provided.defaultProps = {}; // expect: restrict(syntax)

// S1
export function useStatus(id: components["schemas"]["Status"]["id"]) {
  const queryClient = useQueryClient();
  queryClient.setQueryData(["status", id], null); // expect: restrict(feature) @tanstack/query(prefer-query-options)
  void fetch("/api"); // expect: eslint(no-restricted-globals)
  void window.fetch("/api"); // expect: eslint(no-restricted-properties)
  void client;
  void createClient;
  return useQuery({ queryKey: ["status"], queryFn: () => id }); // expect: @tanstack/query(exhaustive-deps) @tanstack/query(prefer-query-options)
}

// S11
export function storedTheme() {
  const theme = localStorage.getItem("my:display:theme"); // expect: restrict(storage)
  window.sessionStorage.removeItem("my:oauth-pending"); // expect: restrict(storage)
  return theme;
}

// S7
export const Route = createFileRoute("/lint-violations")({ // expect: @tanstack/router(create-route-property-order)
  loader: () => undefined,
  beforeLoad: () => undefined,
});
export function load(queryClient: ReturnType<typeof useQueryClient>) {
  return queryClient.ensureQueryData({ queryKey: ["x"], queryFn: () => 1, staleTime: "static" }); // expect: restrict(syntax) restrict(syntax) typescript(no-deprecated)
}

// A7
export const Typed: FC = () => null; // expect: typescript(no-restricted-types)

// A5, A8, A9, D2, D5–D10, Y12, G10
export function Styled({ size }: { size: string }) {
  const el = document.body;
  el.setAttribute("style", "color: red"); // expect: restrict(syntax)
  el.style.cssText = ""; // expect: restrict(syntax) react(immutability)
  el.innerHTML = size; // expect: restrict(html) react(immutability)
  void IndexRoute;
  void useComposeForm;
  void PiStar;
  void tv;
  return (
    <div>
      <div onClick={() => undefined} /> {/* expect: restrict(syntax) */}
      <p dangerouslySetInnerHTML={{ __html: size }} /> {/* expect: @eslint-react(dom-no-dangerously-set-innerhtml) */}
      <p>Hello</p> {/* expect: formatjs(no-literal-string-in-jsx) */}
      <p className="md:flex" /> {/* expect: restrict(classes) better-tailwindcss(no-restricted-classes) */}
      <p className="h-screen" /> {/* expect: restrict(classes) better-tailwindcss(no-restricted-classes) */}
      <p style={{ height: "100vh" }} /> {/* expect: restrict(syntax) */}
      <p className="animate-spin" /> {/* expect: restrict(classes) better-tailwindcss(no-restricted-classes) */}
      <p className="ml-2" /> {/* expect: restrict(classes) better-tailwindcss(no-restricted-classes) */}
      <p className="not-a-class" /> {/* expect: restrict(classes) better-tailwindcss(no-unknown-classes) */}
      <p className={"p-" + size} /> {/* expect: restrict(classes) better-tailwindcss(no-concatenated-classes) */}
      <p className="flex grid" /> {/* expect: restrict(classes) better-tailwindcss(no-conflicting-classes) better-tailwindcss(no-conflicting-classes) */}
      <svg /> {/* expect: restrict(svg) */}
      <a href="https://remote.example/" target="_blank" rel="noreferrer">{size}</a> {/* expect: restrict(syntax) restrict(syntax) */}
    </div>
  );
}

// G5
export function Region({ title }: { title: string }) {
  return (
    <Stack as="section" aria-labelledby="region">
      <Heading level={2} id="region" size="label">{title}</Heading> {/* expect: restrict(pattern) restrict(pattern) */}
    </Stack>
  );
}

// G12
export function More({ label }: { label: string }) {
  return (
    <Button aria-label={label}> {/* expect: restrict(icons) */}
      <MoreIcon />
    </Button>
  );
}
