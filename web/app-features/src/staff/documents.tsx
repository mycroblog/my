// SPDX-License-Identifier: AGPL-3.0-only

// Terms and policies, at /admin/documents (FR-705, ADR-0058 §1): the terms of service, the privacy policy and the
// commerce disclosure, each a door to its page at /admin/documents/<document>, where it is written in Markdown, once,
// in any language, and shown as it is in every locale: a form of one field saved as a whole. What the server refuses
// of it (too long) stands under the field, which takes the focus; an empty text clears it.

import {
  instanceTextLimits,
  loadAdminSettings,
  refusalDetail,
  refusedField,
  useAdminSettings,
  useAdminSettingsWrites,
  type AppData,
  type InstanceTextKey,
} from "@my/app-data";
import {
  BackButton,
  Button,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  SaveState,
  ScreenStack,
  ScreenTitle,
  SettingsGroup,
  SettingsItem,
  SettingsMenu,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useStaffStrings } from "./parts";
import { LeaveDialog, ScreenNotice, type AdminSubProps } from "./settings/parts";

/** The documents, by the last part of their address, and the text each is. */
const documents = [
  { path: "terms", key: "terms" },
  { path: "privacy-policy", key: "privacy_policy" },
  { path: "commerce-disclosure", key: "commerce_disclosure" },
] as const satisfies readonly { path: string; key: InstanceTextKey }[];
type DocumentKey = (typeof documents)[number]["key"];

/** The address of Terms and policies. */
const adminDocumentsPath = "/admin/documents";

/** The address of the page of a document. */
const documentPath = (path: string) => `${adminDocumentsPath}/${path}`;

/** Terms and policies: a door to each document. */
export function AdminDocumentsScreen(): ReactNode {
  const { t } = useStaffStrings();
  return (
    <ScreenStack>
      <ScreenTitle glyph="file">{t("staff.entry.documents")}</ScreenTitle>
      <SettingsMenu label={t("staff.entry.documents")}>
        <SettingsGroup heading={t("staff_settings.documents.group")}>
          {documents.map((d) => (
            <SettingsItem
              key={d.path}
              icon="file"
              label={t("staff_settings.document", { document: d.key })}
              href={documentPath(d.path)}
              count={undefined}
            />
          ))}
        </SettingsGroup>
      </SettingsMenu>
    </ScreenStack>
  );
}

/** Reads the texts, for the route's loader of a document's page. */
export const loadAdminDocument = (data: AppData): Promise<void> => loadAdminSettings(data);

/** The title of the page of the document at `path`; undefined for an address that names none. */
export function useAdminDocumentTitle(path: string): string | undefined {
  const { t } = useStaffStrings();
  const found = documents.find((d) => d.path === path);
  return found === undefined ? undefined : t("staff_settings.document", { document: found.key });
}

/** The form of one document, once its text is read. */
function DocumentForm({
  document,
  body,
  title,
  backButton,
  go,
  onUnsaved,
  leaving,
}: AdminSubProps & { document: DocumentKey; body: string; title: string; backButton: ReactNode }) {
  const { t } = useStaffStrings();
  const id = useId();
  const writes = useAdminSettingsWrites();
  const [base, setBase] = useState(body);
  const [text, setText] = useState(body);
  const [sent, setSent] = useState<
    { kind: "idle" | "sending" | "saved" } | { kind: "refused"; field?: string; reason?: string | undefined }
  >({ kind: "idle" });
  const busy = sent.kind === "sending";
  const unsaved = text !== base;
  const focusNext = useRef(false);
  useEffect(() => {
    if (!focusNext.current) return;
    focusNext.current = false;
    focusRefusedField(id);
  });
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );
  const save = () => {
    if (busy) return;
    if (!unsaved) {
      go(adminDocumentsPath);
      return;
    }
    setSent({ kind: "sending" });
    writes.setText(document, text).then(
      () => {
        setBase(text);
        setSent({ kind: "saved" });
      },
      (error: unknown) => {
        const named = refusedField(error);
        if (named !== undefined) {
          setSent({
            kind: "refused",
            field: t("staff_settings.document.rule", { max: instanceTextLimits[document] }),
          });
          focusNext.current = true;
        } else setSent({ kind: "refused", reason: refusalDetail(error) });
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={title}
        back={backButton}
        foot={
          <SubmitBar
            result={
              sent.kind === "saved" ? (
                <SaveState state="saved" />
              ) : sent.kind === "refused" ? (
                <FormAlert>
                  {sent.reason ??
                    t(sent.field === undefined ? "staff_settings.save.failed" : "staff_settings.save.refused")}
                </FormAlert>
              ) : undefined
            }
          >
            <Button kind="primary" busy={busy} onPress={save}>
              {t("staff_settings.save")}
            </Button>
          </SubmitBar>
        }
      >
        <SettingsSection heading={t("staff_settings.document.text")} about={t("staff_settings.document.about")}>
          <TextField
            id={id}
            name={document}
            label={title}
            labelHidden
            defaultValue={body}
            multiline
            rows={16}
            maxLength={instanceTextLimits[document]}
            error={sent.kind === "refused" ? sent.field : undefined}
            onChange={(value) => {
              setText(value);
              setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
            }}
          />
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}

/** The page of the document at `path`: its form once the texts are read, until then that they load or failed. */
export function AdminDocumentScreen({ path, ...sub }: AdminSubProps & { path: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminSettings();
  const found = documents.find((d) => d.path === path);
  const backButton = (
    <BackButton
      onPress={() => {
        sub.go(adminDocumentsPath);
      }}
    >
      {t("staff.entry.documents")}
    </BackButton>
  );
  const title =
    found === undefined ? t("staff.entry.documents") : t("staff_settings.document", { document: found.key });
  if (found === undefined || read.texts === undefined)
    return (
      <SettingsScreen
        sub
        title={title}
        back={backButton}
        notice={
          <ScreenNotice
            status={found === undefined ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_settings.loading"),
              failed: t("staff_settings.failed"),
              missing: t("staff_settings.missing"),
            }}
          />
        }
      />
    );
  return (
    <DocumentForm
      key={found.key}
      document={found.key}
      body={read.texts[found.key] ?? ""}
      title={title}
      backButton={backButton}
      {...sub}
    />
  );
}
