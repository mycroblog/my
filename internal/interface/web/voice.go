// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// This file serves the shell of the page of a voice chat room (/voice/{room_id}, ADR-0100 §9).
// Like every shell it is the same for every visitor (ADR-0028): a signed-in local user's web
// client loads the room with its token, and the others see that only the local users join,
// with the summary of the room from the client API when the instance shows it. The shell
// carries the link preview of the summary only when the instance shows the summaries of its
// rooms (voice_public_summary) and the room is for every local user; any other room, an
// unknown one, and every room while the voice chat is off get the generic shell with 404, which
// does not reveal which.

// pathVoicePrefix is the prefix of the pages of the rooms.
const pathVoicePrefix = "/voice/"

// VoiceRoomSummary is the summary of a room that its page shows to the visitors who do not see
// it.
type VoiceRoomSummary struct {
	Name, Description string
	// HostName is the display name of the host, or the username without one.
	HostName string
	Ended    bool
}

// VoiceRooms reads the summaries of the rooms for their pages. Summary returns ok false when the
// room does not show its summary, is unknown, or the voice chat is off.
type VoiceRooms func(ctx context.Context, roomID string) (s VoiceRoomSummary, ok bool, err error)

// voiceRoomID returns the room of a page of a room, false for any other path.
func voiceRoomID(p string) (string, bool) {
	id, ok := strings.CutPrefix(p, pathVoicePrefix)
	if !ok || id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// errNoVoiceSummary reports a room whose page shows no summary.
var errNoVoiceSummary = errors.New("web: the room shows no summary")

// serveVoiceRoom serves the shell of the page of a room.
func (h *Handler) serveVoiceRoom(w http.ResponseWriter, r *http.Request, roomID string) {
	ctx := r.Context()
	doc, err := h.renderVoiceRoom(ctx, roomID)
	switch {
	case errors.Is(err, errNoVoiceSummary):
		h.serveHidden(w, r)
	case err != nil:
		h.fail(w, r, err)
	default:
		h.shellHeaders(w.Header())
		doc.serve(w, r)
	}
}

// renderVoiceRoom renders the shell of a room that shows its summary, in the instance's default
// locale: an open room is titled with its name and host and described by its description, and
// an ended one shows only its name and that it ended. The pages of the rooms are never indexed:
// a room is for the moment it is open.
func (h *Handler) renderVoiceRoom(ctx context.Context, roomID string) (document, error) {
	if !uuidPattern.MatchString(roomID) {
		return document{}, errNoVoiceSummary
	}
	s, ok, err := h.o.VoiceRooms(ctx, roomID)
	if err != nil {
		return document{}, err
	}
	if !ok {
		return document{}, errNoVoiceSummary
	}
	lang := h.instanceLocale(ctx)
	l := h.o.Messages.Localizer(lang)
	meta := previewMeta{
		Type: ogTypeWebsite, SiteName: h.instanceName(ctx), URL: "https://" + h.o.Host + pathVoicePrefix + roomID, Locale: ogLocale(lang),
	}
	state := "open"
	if s.Ended {
		meta.Title, state = l.ServerPreviewVoiceRoomEnded(s.Name), "ended"
	} else {
		meta.Title = l.ServerPreviewVoiceRoomOpen(s.HostName, s.Name)
		meta.Description = description(s.Description)
	}
	img, err := h.instanceImage(ctx, lang)
	if err != nil {
		return document{}, err
	}
	meta.Image, meta.Card = img, card(img)
	settings, err := h.settings(ctx)
	if err != nil {
		return document{}, err
	}
	version := []string{"voice", roomID, state, meta.Title, meta.Description, imageVersion(img)}
	return h.renderPreview(ctx, lang, meta, settings.CrawlerPolicy().NoIndex(), version)
}
