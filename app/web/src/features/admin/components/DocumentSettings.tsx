// SPDX-License-Identifier: AGPL-3.0-only
// The terms and policies screen (#489), for manage_settings: the terms of service, the privacy policy and the commerce
// disclosures of the supporter plans (ADR-0072 §13, whose legal correctness the server does not vouch for), each on a
// page of its own (a child route under a LocalNav of the pages, web-client/14-admin.md, how a screen splits): when it
// was saved, a link to its page, and one form (an `h2` section) of a Markdown text area with one save. Each is one
// text, written in any language, that every language shows. Removing a saved text asks first (the APG alert dialog
// pattern, web-client/16-look-states-and-wording.md G22).
import {
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Hint,
  InlineError,
  LocalNav,
  Stack,
  StatusMessage,
  TextArea,
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { DocumentLink } from "../../documents";
import { useUnsavedChangesGuard } from "../../settings";
import { type DocumentKey, documentTextLimit } from "../api/documents";
import { type DocumentEditor, useDocumentEditor } from "../hooks/useDocumentSettings";
import { DateTime, Section, RequirePermission } from "../../moderation";

/** A page of the screen: one document. */
export type DocumentPage = DocumentKey;

/** The pages in the order of the LocalNav: their routes, the names of their links, and what their form says. */
const pages = {
  terms: {
    href: "/admin/documents",
    title: "client:admin.documents.terms",
    viewLabel: "client:admin.documents.view_terms",
    confirmTitle: "client:admin.documents.confirm_remove.terms",
  },
  privacy_policy: {
    href: "/admin/documents/privacy-policy",
    title: "client:admin.documents.privacy_policy",
    viewLabel: "client:admin.documents.view_privacy_policy",
    confirmTitle: "client:admin.documents.confirm_remove.privacy_policy",
  },
  commerce_disclosure: {
    href: "/admin/documents/commerce-disclosure",
    title: "client:admin.documents.commerce_disclosure",
    viewLabel: "client:admin.documents.view_commerce_disclosure",
    confirmTitle: "client:admin.documents.confirm_remove.commerce_disclosure",
  },
} as const satisfies Record<
  DocumentKey,
  { href: string; title: ClientMessageKey; viewLabel: ClientMessageKey; confirmTitle: ClientMessageKey }
>;

const order: readonly DocumentKey[] = ["terms", "privacy_policy", "commerce_disclosure"];

/** One document's page of the screen, with the links to the others above it. */
export function DocumentSettings({ documentKey }: { documentKey: DocumentPage }) {
  const t = useT();
  return (
    <RequirePermission permission="manage_settings">
      <Stack gap="8">
        <LocalNav
          label={t("client:admin.documents.pages")}
          items={order.map((key) => ({
            href: pages[key].href,
            label: t(pages[key].title),
            current: key === documentKey,
          }))}
        />
        <DocumentPageForm documentKey={documentKey} />
      </Stack>
    </RequirePermission>
  );
}

function DocumentPageForm({ documentKey }: { documentKey: DocumentPage }) {
  const t = useT();
  const editor = useDocumentEditor(documentKey);
  const page = pages[documentKey];
  return (
    <>
      <Section surface="card" title={t(page.title)}>
        {documentKey === "commerce_disclosure" && (
          <Hint size="inherit">{t("client:admin.documents.commerce_disclosure.description")}</Hint>
        )}
        <DocumentForm editor={editor} viewLabel={t(page.viewLabel)} confirmTitle={t(page.confirmTitle)} />
      </Section>
      <UnsavedChangesDialog hasChanges={editor.hasChanges} />
    </>
  );
}

type DocumentFormProps = {
  editor: DocumentEditor;
  /** The name of the link to the document's public page. */
  viewLabel: string;
  /** The question before removing the text. */
  confirmTitle: string;
};

/** One document: when it was saved, its text, saving it, and its public page. */
function DocumentForm({ editor, viewLabel, confirmTitle }: DocumentFormProps) {
  const t = useT();
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:admin.documents.characters_over", { count: -remaining })
      : t("client:admin.documents.characters_left", { count: remaining });
  const failed =
    editor.error === undefined ? undefined : t("client:admin.documents.save_failed", { reason: editor.error });
  return (
    <>
      <DescriptionList>
        <DescriptionItem term={t("client:admin.documents.saved_at")}>
          {editor.savedAt === undefined ? (
            t("client:admin.documents.saved_at.never")
          ) : (
            <DateTime value={editor.savedAt} />
          )}
        </DescriptionItem>
      </DescriptionList>
      {editor.pageUrl !== undefined && (
        <p>
          <DocumentLink href={editor.pageUrl}>{viewLabel}</DocumentLink>
        </p>
      )}
      <Stack as="form" gap="4" onSubmit={editor.submit}>
        <TextArea
          label={t("client:admin.documents.body")}
          description={t("client:admin.documents.body.description")}
          value={editor.value}
          onChange={editor.change}
          rows={12}
          characterLimit={documentTextLimit}
          formatRemaining={formatRemaining}
          errorMessage={t("client:admin.documents.too_long", { max: documentTextLimit })}
        />
        {editor.confirmation === null && failed !== undefined && (
          <InlineError size="inherit" alert>
            {failed}
          </InlineError>
        )}
        <div>
          <Button type="submit" variant="primary" isPending={editor.isPending && editor.confirmation === null}>
            {t("client:admin.documents.save")}
          </Button>
        </div>
        <StatusMessage>{editor.status}</StatusMessage>
      </Stack>
      {/* Outside the form: React sends the dialog's own submit event up its component tree, through a portal too. */}
      <ConfirmDialog
        isOpen={editor.confirmation !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) editor.confirmation?.cancel();
        }}
        title={confirmTitle}
        description={t("client:admin.documents.confirm_remove.description")}
        confirmLabel={t("client:admin.documents.confirm_remove.confirm")}
        cancelLabel={t("client:admin.documents.confirm_remove.cancel")}
        tone="danger"
        onConfirm={() => {
          editor.confirmation?.confirm();
        }}
        isPending={editor.isPending}
        {...(editor.confirmation !== null && failed !== undefined ? { errorMessage: failed } : {})}
      />
    </>
  );
}

/**
 * Asks before another page replaces the page with an unsaved text (the APG alert dialog pattern): focus starts
 * on "Keep editing", which stays on the screen like Esc does.
 */
function UnsavedChangesDialog({ hasChanges }: { hasChanges: boolean }) {
  const t = useT();
  const guard = useUnsavedChangesGuard(hasChanges);
  return (
    <ConfirmDialog
      isOpen={guard.isAsking}
      onOpenChange={(isOpen) => {
        if (!isOpen) guard.stay();
      }}
      title={t("client:admin.documents.unsaved.title")}
      description={t("client:admin.documents.unsaved.description")}
      confirmLabel={t("client:admin.documents.unsaved.leave")}
      cancelLabel={t("client:admin.documents.unsaved.stay")}
      onConfirm={guard.leave}
    />
  );
}
