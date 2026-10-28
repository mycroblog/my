// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Cluster, ContentWarning, EmojiText, Heading, Link, MediaGrid, PostContent, Stack, Text } from "@my/ui";
import { useId } from "react";
import { type PlainClientMessageKey, useLocale, useT } from "../../../lib/i18n";
import {
  type RevisionChange,
  type RevisionView,
  type StatusHistoryView,
  useStatusHistory,
} from "../hooks/useStatusHistory";
import { useMediaViewerLabels } from "../hooks/useMediaViewerLabels";
import { useAudioPlayerLabels } from "../hooks/useAudioPlayerLabels";

export type StatusHistoryProps = {
  /** The ID of the post; the route loader has fetched it and its history. */
  id: string;
};

const kindLabels: Readonly<Record<RevisionView["kind"], PlainClientMessageKey | null>> = {
  current: "client:status.history.current",
  original: "client:status.history.original",
  edit: null,
};

const changeLabels: Readonly<Record<RevisionChange, PlainClientMessageKey>> = {
  text: "client:status.history.change.text",
  warning: "client:status.history.change.warning",
  sensitive: "client:status.history.change.sensitive",
  media: "client:status.history.change.media",
};

/**
 * The revisions of a post (FR-109), newest first, under a link back to the post. Each revision is an
 * article named by its heading, the date and time it was made, and says whether it is the current content
 * or the post as first written, and what it changed from the revision before it.
 */
export function StatusHistory({ id }: StatusHistoryProps) {
  const t = useT();
  const history = useStatusHistory(id);
  return (
    <Stack>
      <Box as="p" paddingX="4" paddingBottom="2">
        <Link href={history.postPath}>{t("client:status.history.back")}</Link>
      </Box>
      <Stack as="ol" aria-label={t("client:status.history.list")} divided border="y">
        {history.revisions.map((revision) => (
          <li key={revision.number}>
            <Revision revision={revision} history={history} />
          </li>
        ))}
      </Stack>
    </Stack>
  );
}

type RevisionProps = { revision: RevisionView; history: StatusHistoryView };

function Revision({ revision, history }: RevisionProps) {
  const t = useT();
  const locale = useLocale();
  const headingId = useId();
  const viewerLabels = useMediaViewerLabels();
  const audioLabels = useAudioPlayerLabels();
  const kind = kindLabels[revision.kind];
  const changes =
    revision.changes.length === 0
      ? null
      : t("client:status.history.changes", {
          changes: new Intl.ListFormat(locale, { type: "conjunction" }).format(
            revision.changes.map((change) => t(changeLabels[change])),
          ),
        });
  const body = (
    <PostContent html={revision.html} lang={history.lang} emojis={revision.emojis} onLinkClick={history.onLinkClick} />
  );
  const media =
    revision.media.length > 0 ? (
      <MediaGrid
        items={revision.media}
        sensitive={revision.sensitive}
        noDescriptionLabel={t("client:timeline.media.no_description")}
        revealLabel={t("client:timeline.media.reveal")}
        viewerLabels={viewerLabels}
        audioLabels={audioLabels}
      />
    ) : null;
  return (
    <Stack as="article" aria-labelledby={headingId} gap="2" background="surface" paddingX="4" paddingY="3">
      <Heading level={2} id={headingId}>
        <time dateTime={revision.time.dateTime}>{revision.time.absolute}</time>
      </Heading>
      {(kind !== null || changes !== null) && (
        <Cluster as="p" gap="3" rowGap="1" textSize="sm" tone="muted">
          {kind !== null && (
            <Text weight="medium" tone="fg">
              {t(kind)}
            </Text>
          )}
          {changes !== null && <span>{changes}</span>}
        </Cluster>
      )}
      {revision.contentWarning === null ? (
        <>
          {body}
          {media}
        </>
      ) : (
        <ContentWarning
          summary={<EmojiText text={revision.contentWarning} emojis={revision.emojis} />}
          summaryLang={history.lang}
          toggleLabel={t("client:timeline.content_warning.toggle")}
        >
          {body}
          {media}
        </ContentWarning>
      )}
    </Stack>
  );
}
