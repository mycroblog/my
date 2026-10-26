// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	reportedRoomID     = "01890a5d-ac96-774b-bcce-b302099a6001"
	reportedRevisionID = "01890a5d-ac96-774b-bcce-b302099a6002"
)

// fakeVoiceModeration sees the room reportedRoomID, hosted by reportTargetID, for every
// reporter, and ends it once.
type fakeVoiceModeration struct {
	ended []appvoice.ModeratorEndInput
}

func (f *fakeVoiceModeration) ReportTarget(_ context.Context, _, roomID string) (appvoice.ReportTarget, error) {
	if roomID != reportedRoomID {
		return appvoice.ReportTarget{}, appvoice.ErrNotFound
	}
	return appvoice.ReportTarget{RoomID: roomID, RevisionID: reportedRevisionID, HostAccountID: reportTargetID}, nil
}

func (f *fakeVoiceModeration) EndByModerator(_ context.Context, in appvoice.ModeratorEndInput) error {
	switch {
	case len([]rune(in.Note)) > appvoice.MaxEndNote:
		return appvoice.ErrInvalidEndNote
	case in.RoomID != reportedRoomID || len(f.ended) > 0:
		return appvoice.ErrNotFound
	}
	f.ended = append(f.ended, in)
	return nil
}

func (f *fakeVoiceModeration) RoomRecords(_ context.Context, roomID string) (appvoice.RoomRecords, error) {
	if roomID != reportedRoomID {
		return appvoice.RoomRecords{}, appvoice.ErrNotFound
	}
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	return appvoice.RoomRecords{
		RoomRecord: appvoice.RoomRecord{
			Room:   appvoice.Room{ID: roomID, OpenedAt: at},
			Ending: &appvoice.Ending{RoomID: roomID, Reason: voice.EndModerator, EndedByAccountID: aliceAccountID, EndedAt: at.Add(time.Hour), Note: "spam"},
		},
		Revisions: []appvoice.Revision{{
			ID: reportedRevisionID, RoomID: roomID, RevisedByAccountID: reportTargetID, RevisedAt: at,
			Settings: voice.Settings{Name: "Room", Visibility: voice.VisibilityLocal, Speakers: voice.SpeakersManual, Hashtags: []voice.Hashtag{{Name: "Go", NameNormalized: "go"}}},
		}},
		Participations: []appvoice.ParticipationRecord{
			{Participation: appvoice.Participation{ID: "p1", RoomID: roomID, AccountID: reportTargetID, JoinedAt: at},
				End: &appvoice.ParticipationEnd{Reason: voice.RoomEnded, EndedAt: at.Add(time.Hour)}},
		},
		Bans: []appvoice.Ban{{ID: "b1", RoomID: roomID, AccountID: bobAccountID, BannedByAccountID: reportTargetID, BannedAt: at}},
	}, nil
}

func TestCreateReportOfAVoiceRoom(t *testing.T) {
	reports := &fakeReports{}
	e := newReportsEnvWith(t, reports, &fakeVoiceModeration{})
	res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "voice_room_id": reportedRoomID, "category": "spam", "forward": true})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got := decodeJSON[clientapi.Report](t, res)
	if got.VoiceRoomId == nil || *got.VoiceRoomId != reportedRoomID {
		t.Errorf("report = %+v, want the room", got)
	}
	if in := reports.got[0]; in.VoiceRoom == nil || *in.VoiceRoom != (moderation.ReportVoiceRoom{RoomID: reportedRoomID, RevisionID: reportedRevisionID}) ||
		in.TargetAccountID != reportTargetID {
		t.Errorf("filed %+v, want the room, its revision, and its host", in)
	}
	for _, tc := range []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"a room that the user does not see", map[string]any{"account_id": reportTargetID, "voice_room_id": reportOne, "category": "spam"}, "voice_room_id"},
		{"an account that is not the host", map[string]any{"account_id": aliceAccountID, "voice_room_id": reportedRoomID, "category": "spam"}, "account_id"},
	} {
		res := e.postReport(t, "write", tc.body)
		p := decodeProblem(t, res)
		if res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, tc.field) {
			t.Errorf("%s = %d %+v, want 422 on %s", tc.name, res.StatusCode, p, tc.field)
		}
	}
	if len(reports.got) != 1 {
		t.Errorf("filed %d reports, want the first alone", len(reports.got))
	}

	// Without the voice chat rooms, a report names no room.
	e = newReportsEnv(t, &fakeReports{})
	if res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "voice_room_id": reportedRoomID, "category": "spam"}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("without the rooms = %d, want 422", res.StatusCode)
	}
}

func TestEndVoiceRoomAsModerator(t *testing.T) {
	rooms := &fakeVoiceModeration{}
	e, _ := newModerationEnvWith(t, func(o *clientapi.Options) { o.VoiceModeration = rooms })
	if res := e.sendJSON(t, http.MethodPost, "/admin/voice_rooms/"+reportedRoomID+"/end", "admin-read", `{}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("with admin:read = %d, want 403", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, "/admin/voice_rooms/"+reportedRoomID+"/end", "admin", `{"reason":"spam"}`); res.StatusCode != http.StatusNoContent {
		t.Fatalf("end = %d, want 204", res.StatusCode)
	}
	want := appvoice.ModeratorEndInput{RoomID: reportedRoomID, ByUserID: userOf(aliceAccountID), ByAccountID: aliceAccountID, Note: "spam"}
	if len(rooms.ended) != 1 || rooms.ended[0] != want {
		t.Errorf("ended %+v, want %+v", rooms.ended, want)
	}
	if res := e.sendJSON(t, http.MethodPost, "/admin/voice_rooms/"+reportedRoomID+"/end", "admin", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("end of an ended room = %d, want 404", res.StatusCode)
	}
}

func TestReportDetailShowsTheVoiceRoom(t *testing.T) {
	e, f := newModerationEnvWith(t, func(o *clientapi.Options) { o.VoiceModeration = &fakeVoiceModeration{} })
	f.reports[0].Report.VoiceRoom = &moderation.ReportVoiceRoom{RoomID: reportedRoomID, RevisionID: reportedRevisionID}
	res := e.sendJSON(t, http.MethodGet, "/admin/reports/"+reportOne, "admin-read", "")
	got := decodeJSON[clientapi.AdminReportDetail](t, res)
	r := got.VoiceRoom
	if res.StatusCode != http.StatusOK || got.VoiceRoomId == nil || *got.VoiceRoomId != reportedRoomID || r == nil {
		t.Fatalf("detail = %d %+v, want the room", res.StatusCode, got)
	}
	if r.ReportedRevisionId != reportedRevisionID || len(r.Revisions) != 1 || r.Revisions[0].Name != "Room" || r.Revisions[0].Hashtags[0] != "Go" ||
		len(r.Participations) != 1 || r.Participations[0].LeaveReason == nil || *r.Participations[0].LeaveReason != "room_ended" ||
		len(r.Bans) != 1 || r.Bans[0].AccountId != bobAccountID || r.EndNote == nil || *r.EndNote != "spam" || r.EndReason == nil || *r.EndReason != "moderator" {
		t.Errorf("room = %+v", r)
	}
	res = e.sendJSON(t, http.MethodGet, "/admin/reports/"+reportTwo, "admin-read", "")
	if got := decodeJSON[clientapi.AdminReportDetail](t, res); got.VoiceRoom != nil || got.VoiceRoomId != nil {
		t.Errorf("a report of no room = %+v", got)
	}
}
