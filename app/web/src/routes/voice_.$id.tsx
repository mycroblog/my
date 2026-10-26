// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { VoiceRoomView, voiceRoomQueryOptions, voiceRoomSummaryQueryOptions } from "../features/voice";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/** The room's ID (api/spec VoiceRoomId). */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * The page of a voice chat room (ADR-0100 §9), which the start post and the notifications link: before joining for a
 * signed-in user who sees the room, its summary for everyone else. It needs no session.
 */
export const Route = createFileRoute("/voice_/$id")({
  params: { parse: (params) => paramsSchema.parse(params) },
  loader: async ({ context: { queryClient, api, session }, params: { id } }) => {
    await session.restore();
    // A failure does not fail the page: the page tells an ended, hidden, or unknown room from the summary.
    const room =
      session.status === "signed-in" ? await queryClient.query({ ...voiceRoomQueryOptions(api, id), staleTime: Infinity }).catch(() => undefined) : undefined;
    if (room === undefined) await queryClient.query({ ...voiceRoomSummaryQueryOptions(api, id), staleTime: Infinity }).catch(() => undefined);
  },
  component: VoiceRoomPage,
  errorComponent: VoiceRoomError,
});

function VoiceRoomPage() {
  const { id } = Route.useParams();
  // A new room is a new page: nothing of the previous one carries over (web-client/README.md R10).
  return <VoiceRoomView key={id} id={id} renderHeading={(text) => <PageHeading>{text}</PageHeading>} />;
}

/** The page keeps a heading when it fails (web-client/16-look-states-and-wording.md G16). */
function VoiceRoomError(props: ErrorComponentProps) {
  const t = useT();
  return <RouteError {...props} heading={t("client:voice.room.title")} />;
}
