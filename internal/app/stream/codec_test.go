// SPDX-License-Identifier: AGPL-3.0-only

package stream

import (
	"errors"
	"strings"
	"testing"
)

func TestSignalRoundTrips(t *testing.T) {
	for _, e := range []Event{
		{Kind: KindHomeEntry, EntryID: "0199f0a8-0000-7000-8000-000000000001"},
		{Kind: KindNotifications, AccountID: "0199f0a8-0000-7000-8000-000000000002"},
		{Kind: KindAnnouncement, AnnouncementID: "0199f0a8-0000-7000-8000-000000000003"},
		{Kind: KindVoiceRoom, RoomID: "r1"},
		{Kind: KindVoiceSignal, RoomID: "r1", MessageID: "m1"},
		{Kind: KindVoiceMic, RoomID: "r1", ParticipantID: "a1", On: true},
		{Kind: KindVoiceMic, RoomID: "r1", ParticipantID: "a1", On: false},
		{Kind: KindVoiceReaction, RoomID: "r1", ParticipantID: "a1", Emoji: ":blobcat:"},
		{Kind: KindVoiceMute, RoomID: "r1", SessionID: "s1", By: "a2"},
	} {
		b, err := encodeSignal("p1", e)
		if err != nil {
			t.Fatalf("encode %+v: %v", e, err)
		}
		origin, got, err := decodeSignal(b)
		if err != nil || origin != "p1" || got != e {
			t.Errorf("decode %s = %q, %+v, %v; want p1, %+v", b, origin, got, err, e)
		}
	}
	b, _ := encodeSignal("p1", Event{Kind: KindHomeEntry, EntryID: "0199f0a8-0000-7000-8000-000000000001"})
	if want := `{"v":1,"o":"p1","k":"home_entry","e":"0199f0a8-0000-7000-8000-000000000001"}`; string(b) != want {
		t.Errorf("payload = %s, want %s", b, want)
	}
	b, _ = encodeSignal("p1", Event{Kind: KindNotifications, AccountID: "0199f0a8-0000-7000-8000-000000000002"})
	if want := `{"v":1,"o":"p1","k":"notifications","a":"0199f0a8-0000-7000-8000-000000000002"}`; string(b) != want {
		t.Errorf("payload = %s, want %s", b, want)
	}
	b, _ = encodeSignal("p1", Event{Kind: KindAnnouncement, AnnouncementID: "0199f0a8-0000-7000-8000-000000000003"})
	if want := `{"v":1,"o":"p1","k":"announcement","n":"0199f0a8-0000-7000-8000-000000000003"}`; string(b) != want {
		t.Errorf("payload = %s, want %s", b, want)
	}
	for e, want := range map[Event]string{
		{Kind: KindVoiceRoom, RoomID: "r1"}:                                      `{"v":1,"o":"p1","k":"voice_room","r":"r1"}`,
		{Kind: KindVoiceSignal, RoomID: "r1", MessageID: "m1"}:                   `{"v":1,"o":"p1","k":"voice_signal","r":"r1","m":"m1"}`,
		{Kind: KindVoiceMic, RoomID: "r1", ParticipantID: "a1"}:                  `{"v":1,"o":"p1","k":"voice_mic","r":"r1","p":"a1","on":false}`,
		{Kind: KindVoiceReaction, RoomID: "r1", ParticipantID: "a1", Emoji: "👏"}: `{"v":1,"o":"p1","k":"voice_reaction","e":"👏","r":"r1","p":"a1"}`,
		{Kind: KindVoiceMute, RoomID: "r1", SessionID: "s1", By: "a2"}:           `{"v":1,"o":"p1","k":"voice_mute","r":"r1","s":"s1","b":"a2"}`,
	} {
		if b, _ := encodeSignal("p1", e); string(b) != want {
			t.Errorf("payload = %s, want %s", b, want)
		}
	}
}

func TestSignalSizeLimit(t *testing.T) {
	if _, err := encodeSignal("p1", Event{Kind: KindHomeEntry, EntryID: strings.Repeat("x", MaxPayloadSize)}); err == nil {
		t.Error("a signal over MaxPayloadSize was encoded")
	}
}

func TestSignalDecodeRejects(t *testing.T) {
	for payload, want := range map[string]error{
		`{"v":2,"o":"p1","k":"home_entry","e":"0199f0a8-0000-7000-8000-000000000001"}`:                  errUnknownVersion,
		`{"o":"p1","k":"home_entry","e":"0199f0a8-0000-7000-8000-000000000001"}`:                        errUnknownVersion,
		`{"v":1,"o":"p1","k":"home_entry"}`:                                                             errMalformedSignal,
		`{"v":1,"o":"p1","k":"notifications","e":"0199f0a8-0000-7000-8000-000000000001"}`:               errMalformedSignal,
		`{"v":1,"o":"p1","k":"reaction","e":"0199f0a8-0000-7000-8000-000000000001"}`:                    errMalformedSignal,
		`{"v":1,"o":"p1","k":"announcement","a":"0199f0a8-0000-7000-8000-000000000002"}`:                errMalformedSignal,
		`{"v":1,"o":"p1","k":"voice_room"}`:                                                             errMalformedSignal,
		`{"v":1,"o":"p1","k":"voice_signal","r":"r1"}`:                                                  errMalformedSignal,
		`{"v":1,"o":"p1","k":"voice_mic","r":"r1","p":"a1"}`:                                            errMalformedSignal,
		`{"v":1,"o":"p1","k":"voice_reaction","r":"r1","p":"a1"}`:                                       errMalformedSignal,
		`{"v":1,"o":"p1","k":"voice_reaction","r":"r1","p":"a1","e":"` + strings.Repeat("x", 65) + `"}`: errMalformedSignal,
		`{"v":1,"o":"p1","k":"voice_mute","r":"r1","s":"s1"}`:                                           errMalformedSignal,
		`not json`: errMalformedSignal,
	} {
		if _, _, err := decodeSignal([]byte(payload)); !errors.Is(err, want) {
			t.Errorf("decode %s: %v, want %v", payload, err, want)
		}
	}
}
