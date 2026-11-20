// SPDX-License-Identifier: AGPL-3.0-only

// A new action on a remote domain, at /moderation/domain-actions/new (FR-702, FR-716, FR-717): the domain (filled in
// where the address names it), what the action does, how long it lasts (ten years at most), the reason others may read
// where the server publishes its domain actions, the note only the moderators read, and whether a published list hides
// part of the domain. A suspend cuts every follow between the people here and the domain's accounts for good: before it
// is issued, the confirmation says how many, and asks the reader to type the domain. The action's page opens once it is
// issued.

import {
  domainActionKinds,
  domainCommentLimit,
  domainLimit,
  refusedMember,
  useDomainActionWrites,
  useDomainSuspendImpact,
  type DomainActionKind,
} from "@my/app-data";
import {
  BackButton,
  Button,
  CheckField,
  ConfirmDialog,
  FormAlert,
  ModalPresence,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  TextField,
  type SelectOption,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { useStaffStrings, type T } from "../parts";
import { domainKindWords, LabelledSelect, refusalWords, tied } from "./parts";

/** How long an action may last, in days, as the choice offers it: until lifted (0), up to ten years. */
const lengths = [0, 1, 7, 30, 90, 365, 3650] as const;

/** The choice of how long an action lasts, its value the days ("0" for until lifted). */
function lengthOptions(t: T): SelectOption[] {
  return lengths.map((days) => ({
    value: String(days),
    label: days === 0 ? t("staff_domains.until_lifted") : t("staff_domains.length.days", { count: days }),
  }));
}

/** What the form holds. */
type Draft = {
  domain: string;
  kind: DomainActionKind;
  days: string;
  publicComment: string;
  privateNote: string;
  obfuscate: boolean;
};

/** The request of `d`, its texts trimmed and left out when empty. */
const requestOf = (d: Draft) => ({
  domain: d.domain.trim(),
  kind: d.kind,
  obfuscate: d.obfuscate,
  ...(d.publicComment.trim() === "" ? {} : { public_comment: d.publicComment.trim() }),
  ...(d.privateNote.trim() === "" ? {} : { private_note: d.privateNote.trim() }),
  ...(d.days === "0" ? {} : { expires_in: Number(d.days) * 86_400 }),
});

export type NewDomainActionProps = {
  /** The domain the form starts with (the address's `domain`). */
  domain?: string | undefined;
  onBack: () => void;
  /** Opens the action of `id`, once issued. */
  onIssued: (id: string) => void;
};

/** The form of a new domain action. */
export function NewDomainActionScreen({ domain = "", onBack, onIssued }: NewDomainActionProps): ReactNode {
  const { t } = useStaffStrings();
  const { issue } = useDomainActionWrites();
  const id = useId();
  const ids = { domain: `${id}-domain`, public: `${id}-public`, private: `${id}-private` };
  const [draft, setDraft] = useState<Draft>({
    domain,
    kind: "suspend",
    days: "0",
    publicComment: "",
    privateNote: "",
    obfuscate: false,
  });
  const [faults, setFaults] = useState<Partial<Record<string, string>>>({});
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const change = (next: Partial<Draft>) => {
    setDraft((d) => ({ ...d, ...next }));
    setAlert(undefined);
  };
  const send = () => {
    setBusy(true);
    issue(requestOf(draft)).then(
      (action) => {
        setBusy(false);
        setAsking(false);
        onIssued(action.id);
      },
      (e: unknown) => {
        setBusy(false);
        setAsking(false);
        const member = refusedMember(e);
        const field = { domain: ids.domain, public_comment: ids.public, private_note: ids.private }[
          member?.member ?? ""
        ];
        if (member !== undefined && field !== undefined) {
          setFaults({
            [member.member]: member.member === "domain" ? t("staff_domains.form.domain.invalid") : member.said,
          });
          requestAnimationFrame(() => document.getElementById(field)?.focus());
        } else setAlert(refusalWords(t, e));
      },
    );
  };
  const submit = () => {
    if (busy) return;
    setFaults({});
    setAlert(undefined);
    if (draft.domain.trim() === "") {
      setFaults({ domain: t("staff_domains.form.domain.blank") });
      document.getElementById(ids.domain)?.focus();
      return;
    }
    // A suspend asks first: the follows it cuts do not come back when it is lifted.
    if (draft.kind === "suspend") setAsking(true);
    else send();
  };
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_domains.actions.back")}</BackButton>}>
        {t("staff_domains.actions.new")}
      </ScreenTitle>
      <Stack
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
          defaultValue={domain}
          autoCapitalize="none"
          maxLength={domainLimit}
          error={faults["domain"]}
          onChange={(value) => {
            change({ domain: value });
          }}
        />
        <LabelledSelect
          label={t("staff_domains.form.kind")}
          options={domainActionKinds.map((k) => ({ value: k, label: domainKindWords(t, k) }))}
          value={draft.kind}
          onChange={(kind) => {
            change({ kind: domainActionKinds.find((k) => k === kind) ?? "suspend" });
          }}
        />
        <LabelledSelect
          label={t("staff_domains.form.length")}
          options={lengthOptions(t)}
          value={draft.days}
          onChange={(days) => {
            change({ days });
          }}
        />
        <TextField
          id={ids.public}
          name="public_comment"
          label={t("staff_domains.form.public_comment")}
          multiline
          rows={2}
          maxLength={domainCommentLimit}
          error={faults["public_comment"]}
          onChange={(value) => {
            change({ publicComment: value });
          }}
        />
        <TextField
          id={ids.private}
          name="private_note"
          label={t("staff_domains.form.private_note")}
          multiline
          rows={2}
          maxLength={domainCommentLimit}
          error={faults["private_note"]}
          onChange={(value) => {
            change({ privateNote: value });
          }}
        />
        <CheckField
          id={`${id}-obfuscate`}
          name="obfuscate"
          value="true"
          label={t("staff_domains.form.obfuscate")}
          checked={draft.obfuscate}
          onChange={(obfuscate) => {
            change({ obfuscate });
          }}
          touch
        />
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button type="submit" kind="primary" busy={busy && !asking}>
            {t("staff_domains.form.submit", { kind: draft.kind })}
          </Button>
        </Row>
      </Stack>
      <ModalPresence>
        {asking && (
          <SuspendConfirm
            domain={draft.domain.trim()}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={send}
          />
        )}
      </ModalPresence>
    </ScreenStack>
  );
}

/**
 * The confirmation of a suspend of `domain`: what it stops, the follows it cuts for good as the server counts them now
 * (while it counts, that it does; where it could not, that it cuts them, uncounted), and the domain to type.
 */
export function SuspendConfirm({
  domain,
  busy,
  onCancel,
  onConfirm,
  layer,
}: {
  domain: string;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
}) {
  const { t } = useStaffStrings();
  const impact = useDomainSuspendImpact(domain);
  const cut =
    impact.value === undefined
      ? t(impact.status === "pending" ? "staff_domains.suspend.counting" : "staff_domains.suspend.uncounted")
      : tied(
          t("staff_domains.suspend.cuts", {
            follows: impact.value.follows,
            people: impact.value.local_accounts,
          }),
        );
  return (
    <ConfirmDialog
      title={t("staff_domains.suspend.title", { domain })}
      cancel={t("staff_domains.cancel")}
      confirm={t("staff_domains.suspend.confirm")}
      phrase={domain}
      busy={busy}
      // The suspend waits for the count of what it ends; where the count failed, the dialog says so and lets it go.
      pending={impact.status === "pending"}
      layer={layer}
      onCancel={onCancel}
      onConfirm={onConfirm}
    >
      <Stack gap="entry-line">
        <span>{t("staff_domains.suspend.stops", { domain })}</span>
        <span role="status">{cut}</span>
      </Stack>
    </ConfirmDialog>
  );
}
