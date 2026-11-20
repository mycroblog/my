// SPDX-License-Identifier: AGPL-3.0-only

// The pages under the about page that the server's documents and its link card crawler's are (FR-705, ADR-0072 §13),
// for everyone, signed in or not: each named after what it holds and led by the way back to the about page, but where
// the server has not published the document, whose sentence offers that way itself.

import { aboutPagePath } from "@my/app-components";
import { useAboutInstance, useInstanceDocument, type InstanceDocumentKind } from "@my/app-data";
import { AboutBack, AboutWaiting, CrawlerView, DocumentView, useAboutTitle } from "@my/app-features";
import { useNavigate } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { ScreenHeading } from "./shell/heading";

/** The way back to the about page. */
function Back(): ReactNode {
  const navigate = useNavigate();
  return (
    <AboutBack
      onPress={() => {
        void navigate({ to: aboutPagePath });
      }}
    />
  );
}

/** The page of the server's document of `kind`. */
export function DocumentPage({ kind }: { kind: InstanceDocumentKind }): ReactNode {
  const read = useInstanceDocument(kind);
  return (
    <>
      <ScreenHeading>{useAboutTitle(kind)}</ScreenHeading>
      {read.status !== "unpublished" && <Back />}
      <DocumentView kind={kind} read={read} />
    </>
  );
}

/** The page of the server's link card crawler, once the instance is read. */
export function CrawlerPage(): ReactNode {
  const { instance, retry } = useAboutInstance();
  return (
    <>
      <ScreenHeading>{useAboutTitle("crawler")}</ScreenHeading>
      <Back />
      {instance !== undefined ? <CrawlerView instance={instance} /> : <AboutWaiting retry={retry} />}
    </>
  );
}
