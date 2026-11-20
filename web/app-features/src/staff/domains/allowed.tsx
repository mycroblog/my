// SPDX-License-Identifier: AGPL-3.0-only

// The allowlist of federation, at /moderation/domain-actions/allowed (ADR-0159 §1, §6): the domains this server
// federates with while its federation is limited to them, by domain, each with the moderators' comment and when it was
// allowed; allowing a domain (which covers its subdomains), and removing the domains picked, which asks first: while the
// limit is on, a domain removed is cut off at once and its follows end for good, as a suspend ends them.

import { domainCommentLimit, domainLimit, refusedMember, useDomainAllows, useDomainAllowWrites } from "@my/app-data";
import {
  BackButton,
  Button,
  ConfirmDialog,
  FeedNotice,
  FormAlert,
  ModalPresence,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  TextField,
  type RecordRow,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { ReadNotice, useMoment, useStaffStrings, useSaid } from "../parts";
import { refusalWords, Section, tied } from "./parts";

/** The allowlist: allowing a domain, and the domains allowed, which the reader removes. */
export function DomainAllowsScreen({ onBack }: { onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const read = useDomainAllows();
  const moment = useMoment();
  const { remove } = useDomainAllowWrites();
  const { say, status } = useSaid();
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set());
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const back = <BackButton onPress={onBack}>{t("staff_domains.actions.back")}</BackButton>;
  const title = <ScreenTitle back={back}>{t("staff_domains.allows")}</ScreenTitle>;
  if (read.forbidden) return <FeedNotice lead>{t("staff_domains.forbidden")}</FeedNotice>;
  const removeAll = () => {
    const domains = [...picked];
    setBusy(true);
    // One at a time, each its own record in the audit log; those removed stay removed if a later one fails.
    void domains
      .reduce<Promise<number>>(
        (done, domain) => done.then(async (n) => remove(domain).then(() => n + 1)),
        Promise.resolve(0),
      )
      .then(
        (n) => {
          setBusy(false);
          setAsking(false);
          setPicked(new Set());
          say(tied(t("staff_domains.allows.removed", { count: n })));
        },
        (e: unknown) => {
          setBusy(false);
          setAsking(false);
          say(refusalWords(t, e), true);
        },
      );
  };
  const rows = (read.value ?? []).map((a): RecordRow<"allowed"> => ({
    id: a.domain,
    head: a.domain,
    caption: a.comment === "" ? undefined : a.comment,
    cells: { allowed: moment(a.created_at) },
  }));
  return (
    <ScreenStack>
      {title}
      <AllowForm
        onAllowed={(domain) => {
          say(t("staff_domains.allows.added", { domain }));
        }}
      />
      {status}
      <Section title={t("staff_domains.allows.list")}>
        {read.value === undefined ? (
          <ReadNotice
            status={read.status === "error" ? "error" : "pending"}
            loading={t("staff_domains.allows.loading")}
            failed={t("staff_domains.allows.failed")}
            retry={read.refetch}
          />
        ) : (
          <RecordTable
            label={t("staff_domains.allows.list")}
            head={{ label: t("staff_domains.allows.column.domain") }}
            columns={[{ key: "allowed", label: t("staff_domains.allows.column.allowed"), kind: "fit", bare: true }]}
            rows={rows}
            selection={{
              selected: picked,
              onChange: setPicked,
              actions: [
                {
                  id: "remove",
                  label: t("staff_domains.allows.remove"),
                  busy,
                  onPress: () => {
                    setAsking(true);
                  },
                },
              ],
            }}
            empty={<FeedNotice start>{t("staff_domains.allows.empty")}</FeedNotice>}
          />
        )}
      </Section>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={tied(t("staff_domains.allows.remove.title", { count: picked.size, domain: [...picked][0] ?? "" }))}
            cancel={t("staff_domains.cancel")}
            confirm={t("staff_domains.allows.remove.confirm")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={removeAll}
          >
            {t("staff_domains.allows.remove.about", { count: picked.size })}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </ScreenStack>
  );
}

/** Allowing a domain, with a comment for the moderators; the form empties once it is allowed. */
function AllowForm({ onAllowed }: { onAllowed: (domain: string) => void }) {
  const { t } = useStaffStrings();
  const { add } = useDomainAllowWrites();
  const id = useId();
  const ids = { domain: `${id}-domain`, comment: `${id}-comment` };
  const [round, setRound] = useState(0);
  const [domain, setDomain] = useState("");
  const [comment, setComment] = useState("");
  const [faults, setFaults] = useState<Partial<Record<string, string>>>({});
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const submit = () => {
    if (busy) return;
    setFaults({});
    setAlert(undefined);
    if (domain.trim() === "") {
      setFaults({ domain: t("staff_domains.form.domain.blank") });
      document.getElementById(ids.domain)?.focus();
      return;
    }
    setBusy(true);
    add(domain.trim(), comment).then(
      (allow) => {
        setBusy(false);
        setDomain("");
        setComment("");
        // New fields, empty, for the next domain.
        setRound((n) => n + 1);
        onAllowed(allow.domain);
      },
      (e: unknown) => {
        setBusy(false);
        const member = refusedMember(e);
        const status = (e as { status?: unknown } | null)?.status;
        if (status === 400 || member?.member === "domain") {
          setFaults({ domain: t("staff_domains.form.domain.invalid") });
          document.getElementById(ids.domain)?.focus();
        } else if (member?.member === "comment") {
          setFaults({ comment: member.said });
          document.getElementById(ids.comment)?.focus();
        } else if (member?.member === "domains") setAlert(t("staff_domains.allows.full"));
        else setAlert(refusalWords(t, e));
      },
    );
  };
  return (
    <Section title={t("staff_domains.allows.add")}>
      <Stack
        key={round}
        gap="screen-field-gap"
        as={
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          />
        }
      >
        <TextField
          id={ids.domain}
          name="domain"
          label={t("staff_domains.form.domain")}
          hint={t("staff_domains.form.domain.hint")}
          autoCapitalize="none"
          maxLength={domainLimit}
          error={faults["domain"]}
          onChange={setDomain}
        />
        <TextField
          id={ids.comment}
          name="comment"
          label={t("staff_domains.allows.comment")}
          multiline
          rows={2}
          maxLength={domainCommentLimit}
          error={faults["comment"]}
          onChange={setComment}
        />
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button type="submit" kind="primary" busy={busy}>
            {t("staff_domains.allows.submit")}
          </Button>
        </Row>
      </Stack>
    </Section>
  );
}
