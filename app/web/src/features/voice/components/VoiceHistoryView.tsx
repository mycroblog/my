// SPDX-License-Identifier: AGPL-3.0-only
// The voice chat history of the settings (ADR-0100 §13, 回答 32): the rooms the user hosted, newest first, each with
// its name (the host's text), when it opened and ended, and why it ended; an open one says it is still open.
import { Button, Card, DescriptionItem, DescriptionList, EmptyState, Heading, Hint, RegionError, Spinner, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { type DateView, useVoiceHistory, type VoiceHistoryRow } from "../hooks/useVoiceHistory";

function DateTimeText({ value }: { value: DateView }) {
  return <time dateTime={value.dateTime}>{value.text}</time>;
}

export function VoiceHistoryView() {
  const t = useT();
  const history = useVoiceHistory();
  switch (history.status) {
    case "unavailable":
      return <EmptyState heading={t("client:voice.unavailable")} />;
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:voice.list.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError heading={t("client:voice.list.failed")} reason={history.reason} retryLabel={t("client:voice.list.retry")} onRetry={history.retry} isRetrying={history.isRetrying} />
      );
    case "ready":
      return (
        <Stack gap="4">
          <Hint size="inherit">{t("client:voice.history.description")}</Hint>
          {history.rows.length === 0 ? (
            <EmptyState heading={t("client:voice.history.empty")} />
          ) : (
            <Stack as="ul" gap="3">
              {history.rows.map((row) => (
                <HistoryRow key={row.id} row={row} />
              ))}
            </Stack>
          )}
          {history.moreError !== undefined && <RegionError heading={t("client:voice.list.more_failed")} reason={history.moreError} />}
          {history.hasMore && (
            <div>
              <Button onPress={history.loadMore} isPending={history.isLoadingMore}>
                {t("client:voice.list.more")}
              </Button>
            </div>
          )}
        </Stack>
      );
  }
}

function HistoryRow({ row }: { row: VoiceHistoryRow }) {
  const t = useT();
  return (
    <Card as="li" padding="4">
      <Stack gap="3">
        <Heading level={2} size="subsection" wrap="anywhere">
          <span dir="auto">{row.name}</span>
        </Heading>
        <DescriptionList columns={3}>
          <DescriptionItem term={t("client:voice.history.opened")}>
            <DateTimeText value={row.opened} />
          </DescriptionItem>
          <DescriptionItem term={t("client:voice.history.ended")}>
            {row.ended === undefined ? t("client:voice.history.still_open") : <DateTimeText value={row.ended} />}
          </DescriptionItem>
          {row.reason !== undefined && <DescriptionItem term={t("client:voice.history.reason")}>{t(row.reason)}</DescriptionItem>}
        </DescriptionList>
      </Stack>
    </Card>
  );
}
