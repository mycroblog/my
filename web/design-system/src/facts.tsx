// SPDX-License-Identifier: AGPL-3.0-only

// The facts of a record and the figures of a server's state, as the moderation, administration and operations screens
// show them (ADR-0058).
//
// The facts of a record (an account's registration, a webhook's endpoint) are terms and their values: where the list
// has more room than a phone's column, the terms down the start, as wide as the longest of them up to a limit, each
// value beside its term; on a phone, each term over its value, so that a long value (an ID, a key) has the whole line.
// Terms are muted, at the caption's size; values are the text's. A value in the code's face is one that is copied or
// compared character by character (an IP address, an endpoint), and breaks after a slash, never inside a word that
// fits (code-text.tsx), isolated left to right, as code reads in a right-to-left page too. A fact may have an action
// after its value (the button that copies it). A value that is private (an email address, an IP address) is shown only on demand
// (ADR-0058 §9): in its place, Show, which asks the server for it, spins while it is asked, and gives the focus to the
// value once it comes, so that a keyboard and a screen reader land on what they asked for. Every value so shown takes
// one look, the open eye before it and the code's face, apart from the public ones.
//
// The figures that sum up a server's state (open reports, pending registrations, the depth of a queue) are tiles,
// each its number, large, over its label, as many to a row as fit. A figure that leads where it is dealt with stands
// on the raised surface, which is what can be pressed; one that leads nowhere stands on the page, its words lined up
// with what stands above it, as tall as a tile.
// A screen reader reads each as its number and its label, a link where it leads somewhere.
//
// The app formats every figure and gives every word but Show.

import { useEffect, useId, useRef, useState, type HTMLAttributes, type ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { codeRuns } from "./code-text";
import { Icon } from "./icons";
import { Row, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";

/**
 * The frame of the facts, which knows its own width: narrow, one column, each term over its value; wide, the terms'
 * column as wide as the longest term up to a limit, the values' the rest.
 */
function FactFrame({ className, ...rest }: HTMLAttributes<HTMLElement> & { className: string }) {
  return (
    <div className="@container">
      <dl
        {...rest}
        className={`${className} grid grid-cols-[minmax(0,1fr)] gap-y-screen-row-gap @min-[30rem]:grid-cols-[fit-content(12rem)_minmax(0,1fr)] @min-[30rem]:gap-x-shell-edge`}
      />
    </div>
  );
}

/** A fact: its term and its value, on the frame's columns; narrow, the term over the value, a hair apart. */
function FactLine({ children }: { children: ReactNode }) {
  return (
    <div className="col-span-full grid grid-cols-subgrid items-baseline gap-y-[0.125rem] @min-[30rem]:gap-y-0">
      {children}
    </div>
  );
}

const facts = tv({
  slots: {
    root: "text-body text-text",
    term: "text-caption text-muted-text [word-break:auto-phrase]",
    value: "[word-break:auto-phrase]",
    // A value revealed, after the open eye, in the code's face as every private value is; it takes the focus a
    // keyboard gave Show, with the ring a keyboard's focus shows.
    revealed: "rounded-control font-mono outline-hidden focus-visible:shadow-focus",
    eye: "text-muted-text",
  },
});

/** A fact of a record: its term, and its value, shown or kept until the reader asks for it. */
export type Fact = {
  /** What it is (Email, Registered from, Endpoint). */
  term: string;
  /** Its value, as the app writes it; none while it is kept. */
  value?: ReactNode;
  /** Its value is copied or compared character by character (an IP address, an endpoint): in the code's face. */
  code?: boolean | undefined;
  /**
   * The value is private and shown on demand (ADR-0058 §9): `onReveal` asks for it, `busy` while it is asked. Show
   * stands in its place until `value` is given.
   */
  reveal?: { onReveal: () => void; busy?: boolean | undefined } | undefined;
  /** What the reader may do with the value, after it (a button that copies it). */
  action?: ReactNode;
};

/** A value in the code's face, kept in runs where it is text, so that it breaks after a slash, not inside a word. */
const shownValue = (value: ReactNode): ReactNode => (
  <bdi dir="ltr" className="font-mono">
    {typeof value === "string" ? codeRuns(value) : value}
  </bdi>
);

/** A value kept until asked for: Show, then the value, which takes the focus Show had. */
function KeptValue({ fact, termId }: { fact: Fact; termId: string }) {
  const s = facts();
  const { lang } = useReader(undefined);
  const strings = stringsIn(lang);
  const value = useRef<HTMLElement>(null);
  const [asked, setAsked] = useState(false);
  const shown = fact.value !== undefined && fact.value !== null;
  useEffect(() => {
    if (asked && shown) value.current?.focus();
  }, [asked, shown]);
  if (shown)
    return (
      <Row as="span" inline align="baseline" gap="entry-caption">
        <Row as="span" inline rigid className={s.eye()}>
          <Icon name="revealed" place="caption" />
        </Row>
        {/* The value itself takes the focus: the element that holds its text, isolated left to right. */}
        <bdi ref={value} dir="ltr" tabIndex={asked ? -1 : undefined} className={s.revealed()}>
          {typeof fact.value === "string" ? codeRuns(fact.value) : fact.value}
        </bdi>
      </Row>
    );
  return (
    <Row as="span" inline>
      <Button
        kind="secondary"
        size="timeline"
        busy={fact.reveal?.busy === true}
        aria-describedby={termId}
        onPress={() => {
          setAsked(true);
          fact.reveal?.onReveal();
        }}
      >
        <span lang={strings.lang}>{strings.t("facts.reveal")}</span>
      </Button>
    </Row>
  );
}

/** One fact, its term naming the Show that stands for a kept value. */
function FactItem({ fact }: { fact: Fact }) {
  const s = facts();
  const termId = useId();
  return (
    <FactLine>
      <dt id={termId} className={s.term()}>
        {fact.term}
      </dt>
      <dd className={s.value()}>
        {fact.reveal === undefined ? (
          fact.code === true ? (
            shownValue(fact.value)
          ) : (
            fact.value
          )
        ) : (
          <KeptValue fact={fact} termId={termId} />
        )}
        {fact.action !== undefined && fact.action !== null && (
          <>
            {" "}
            <Row as="span" inline rigid>
              {fact.action}
            </Row>
          </>
        )}
      </dd>
    </FactLine>
  );
}

/**
 * The facts of a record, named `label` for assistive technology (Registration, Endpoint): each term with its value,
 * side by side where the list has the room, the term over the value where it has not.
 */
export function FactList({ label, items }: { label: string; items: readonly Fact[] }) {
  const s = facts();
  return (
    <FactFrame aria-label={label} className={s.root()}>
      {items.map((fact) => (
        <FactItem key={fact.term} fact={fact} />
      ))}
    </FactFrame>
  );
}

/**
 * The tiles of the figures, which know their own width: as many columns alike as fit, each at least a few figures
 * and a short label wide, two side by side on a phone.
 */
function TileGrid({ className, ...rest }: HTMLAttributes<HTMLUListElement> & { className: string }) {
  return (
    <ul
      {...rest}
      className={`${className} grid grid-cols-[repeat(auto-fill,minmax(min(100%,8.5rem),1fr))] gap-entry-line`}
    />
  );
}

const tiles = tv({
  slots: {
    list: "list-none p-0",
    // Every tile in the same room, so that a figure on the page lines up with one on a tile.
    tile: "box-border rounded-entry py-screen-row-inline",
    // A tile that leads where the figure is dealt with: the raised surface, answering a press as a control does.
    link: [
      "px-screen-row-inline bg-surface-raised text-text no-underline outline-hidden",
      "data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed data-focus-visible:shadow-focus",
      "[[data-surface-raised]_&]:bg-surface [[data-surface-raised]_&]:data-hovered:bg-surface-hover",
    ],
    figure: "text-title font-strong tabular-nums",
    label: "text-caption text-muted-text [word-break:auto-phrase]",
  },
});

/** A figure of a server's state: its number, as the app formats it, its label, and where it is dealt with. */
export type Figure = {
  /** The number, formatted in the reader's locale (12, 1,204). */
  figure: string;
  /** What it counts (Open reports, Pending registrations). */
  label: string;
  /** Where it is dealt with (the reports, the registrations to review). */
  href?: string | undefined;
};

/**
 * Figures that sum up a server's state, named `label` (This server): each its number over its label, a tile that
 * leads where it is dealt with, or the figure on the page where it leads nowhere.
 */
export function FigureTiles({ label, items }: { label: string; items: readonly Figure[] }) {
  const s = tiles();
  return (
    <TileGrid aria-label={label} className={s.list()}>
      {items.map((item) => {
        const body = (
          <>
            <span className={s.figure()}>{item.figure}</span>
            <span className={s.label()}>{item.label}</span>
          </>
        );
        return (
          <li key={item.label}>
            {item.href === undefined ? (
              <Stack full="block" pack="start" gap="hair" className={s.tile()}>
                {body}
              </Stack>
            ) : (
              <Stack
                full="block"
                pack="start"
                gap="hair"
                as={<Link href={item.href} className={`${s.tile()} ${s.link()}`} />}
              >
                {body}
              </Stack>
            )}
          </li>
        );
      })}
    </TileGrid>
  );
}
