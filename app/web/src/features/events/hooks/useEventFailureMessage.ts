// SPDX-License-Identifier: AGPL-3.0-only
// Why a write to an event failed, in the user's words (web-client/README.md E2): the problem types of the events
// API, and the server's own text for anything else.
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type EventFailure, eventFailure, eventLimits } from "../api/events";

const failureMessages: Partial<Record<EventFailure, ClientMessageKey>> = {
  "not-found": "client:events.error.not_found",
  full: "client:events.error.full",
  closed: "client:events.error.closed",
  unscheduled: "client:events.error.unscheduled",
  "not-eligible": "client:events.error.not_eligible",
  "voice-unavailable": "client:events.error.voice_unavailable",
  "already-invited": "client:events.error.already_invited",
  "invalid-invitee": "client:events.error.invalid_invitee",
  "invalid-coorganizer": "client:events.error.invalid_coorganizer",
  "not-organizer": "client:events.error.not_organizer",
  "organizer-only": "client:events.error.organizer_only",
  "rate-limited": "client:events.error.rate_limited",
};

/** The localized reason of a failed write to an event. */
export function useEventFailureMessage(): (error: unknown) => string {
  const t = useT();
  return (error) => {
    const failure = eventFailure(error);
    if (failure === "too-many-coorganizers") {
      return t("client:events.error.too_many_coorganizers", { max: String(eventLimits.coorganizers) });
    }
    const key = failureMessages[failure];
    return key === undefined ? errorMessage(error) : t(key);
  };
}
