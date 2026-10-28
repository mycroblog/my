// SPDX-License-Identifier: AGPL-3.0-only
// The poll of a post (FR-112): radio buttons or checkboxes and a button to vote while the viewer may, and
// otherwise each option's share of the votes, the viewer's choices marked in words, not by color alone.
import { Poll, PollChoices, PollHint, PollResults, PollSummary } from "@my/ui";
import { useT } from "../../../lib/i18n";
import type { Status } from "../api/status";
import { usePoll } from "../hooks/usePoll";

export type StatusPollProps = {
  status: Status;
  /** Whether the viewer may vote here; false shows the results only (a moderator's page). */
  canVote: boolean;
};

/** The poll of `status`, or nothing without one. */
export function StatusPoll({ status, canVote }: StatusPollProps) {
  const t = useT();
  const view = usePoll(status, canVote);
  if (view === null) return null;
  const { form } = view;
  return (
    <Poll label={t("client:status.poll.label")}>
      {form === null ? (
        <PollResults
          label={t("client:status.poll.results")}
          options={view.options.map((option) => ({ ...option, key: option.index }))}
          emojis={view.emojis}
          lang={view.lang}
          ownChoiceLabel={t("client:status.poll.own_vote")}
        />
      ) : (
        <PollChoices
          label={t("client:status.poll.options")}
          multiple={view.multiple}
          description={view.multiple ? t("client:status.poll.choose_many") : t("client:status.poll.choose_one")}
          options={view.options.map((option) => ({ value: String(option.index), title: option.title }))}
          emojis={view.emojis}
          lang={view.lang}
          selected={form.selected.map(String)}
          onChange={(value, on) => {
            if (view.multiple) form.toggle(Number(value), on);
            else form.choose(Number(value));
          }}
          isDisabled={form.isSubmitting}
          submitLabel={t("client:status.poll.vote")}
          canSubmit={form.canSubmit}
          isPending={form.isSubmitting}
          onSubmit={form.submit}
        />
      )}
      {view.hidden && form === null && <PollHint>{t("client:status.poll.hidden")}</PollHint>}
      <PollSummary
        total={view.total}
        closedLabel={view.closed ? t("client:status.poll.closed") : undefined}
        ends={
          !view.closed && view.ends !== null
            ? {
                dateTime: view.ends.dateTime,
                absolute: view.ends.absolute,
                text: t("client:status.poll.ends", { time: view.ends.relative }),
              }
            : null
        }
        remoteLabel={
          view.remoteHost !== null && form === null
            ? t("client:status.poll.remote_counts", { host: view.remoteHost })
            : undefined
        }
      />
    </Poll>
  );
}
