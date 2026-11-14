// SPDX-License-Identifier: AGPL-3.0-only

package stream

import (
	"encoding/json"
	"errors"
	"fmt"
)

// signalVersion is the version of the payload format that encodeSignal writes.
const signalVersion = 1

// Errors of decodeSignal.
var (
	// errUnknownVersion reports a payload of a version this process does not know, written by
	// a newer process during a rolling deployment; the payload is dropped.
	errUnknownVersion = errors.New("stream: unknown signal version")
	// errMalformedSignal reports a payload that is not a signal of a known kind.
	errMalformedSignal = errors.New("stream: malformed signal")
)

// signal is the wire format of a signal on a Bus (ADR-0050 §1):
// {"v":1,"o":"<process id>","k":"home_entry","e":"<entry id>","u":"<author account id>"} or
// {"v":1,"o":"<process id>","k":"notifications","a":"<account id>"} or
// {"v":1,"o":"<process id>","k":"announcement","n":"<announcement id>"} or
// {"v":1,"o":"<process id>","k":"public_entry","e":"<entry id>","tl":["local","federated"]}
// ("tl" names the timelines that may list the post, at least one), or
// {"v":1,"o":"<process id>","k":"status_deleted","e":"<status id>","u":"<author>","d":"<author domain>","vis":"public"}
// ("d" absent for a local author), and the signals of the
// voice chat rooms (ADR-0101 §3), which carry the room as "r":
// {"v":1,"o":…,"k":"voice_room","r":"<room>"},
// {"v":1,"o":…,"k":"voice_signal","r":…,"m":"<message id>","t":"<session>"},
// {"v":1,"o":…,"k":"voice_mic","r":…,"p":"<participant>","on":true},
// {"v":1,"o":…,"k":"voice_reaction","r":…,"p":…,"e":"<emoji>"}, and
// {"v":1,"o":…,"k":"voice_mute","r":…,"s":"<session>","b":"<by>"}, and the internal signal
// of a TLS certificate (ADR-0163 §6): {"v":1,"o":…,"k":"tls_certificate","h":"<host>","c":"<certificate id>"}.
type signal struct {
	Version   int    `json:"v"`
	Origin    string `json:"o"`
	Kind      Kind   `json:"k"`
	AccountID string `json:"a,omitempty"`
	// E is the entry of a home_entry signal, the conversation of a conversation one, and the
	// emoji of a voice_reaction one.
	E string `json:"e,omitempty"`
	// Author is the "u" of a home_entry signal, the account of its entry, and of a status_deleted
	// one, the author of its status.
	Author string `json:"u,omitempty"`
	// Domain and Visibility are the "d" and the "vis" of a status_deleted signal.
	Domain     string `json:"d,omitempty"`
	Visibility string `json:"vis,omitempty"`
	// AnnouncementID is the "n" of an announcement signal.
	AnnouncementID string `json:"n,omitempty"`
	RoomID         string `json:"r,omitempty"`
	MessageID      string `json:"m,omitempty"`
	To             string `json:"t,omitempty"`
	ParticipantID  string `json:"p,omitempty"`
	On             *bool  `json:"on,omitempty"`
	// Timelines is the "tl" of a public_entry signal.
	Timelines []Timeline `json:"tl,omitempty"`
	SessionID string     `json:"s,omitempty"`
	By        string     `json:"b,omitempty"`
	// Host and CertificateID are the "h" and the "c" of a tls_certificate signal.
	Host          string `json:"h,omitempty"`
	CertificateID string `json:"c,omitempty"`
}

// encodeSignal encodes e as published by the process origin. It fails for a payload over
// MaxPayloadSize, which no ID of the server reaches.
func encodeSignal(origin string, e Event) ([]byte, error) {
	s := signal{
		Version: signalVersion, Origin: origin, Kind: e.Kind, AccountID: e.AccountID, E: e.EntryID, Author: e.AuthorAccountID, AnnouncementID: e.AnnouncementID,
		RoomID: e.RoomID, MessageID: e.MessageID, To: e.To, ParticipantID: e.ParticipantID, SessionID: e.SessionID, By: e.By,
		Host: e.Host, CertificateID: e.CertificateID, Domain: e.AuthorDomain, Visibility: e.Visibility,
	}
	switch e.Kind {
	case KindPublicEntry:
		if e.OnLocal {
			s.Timelines = append(s.Timelines, TimelineLocal)
		}
		if e.OnFederated {
			s.Timelines = append(s.Timelines, TimelineFederated)
		}
	case KindVoiceMic:
		on := e.On
		s.On = &on
	case KindVoiceReaction:
		s.E = e.Emoji
	case KindConversation:
		s.E = e.ConversationID
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxPayloadSize {
		return nil, fmt.Errorf("stream: a %s signal of %d bytes exceeds %d bytes", e.Kind, len(b), MaxPayloadSize)
	}
	return b, nil
}

// decodeSignal decodes a payload into the signal and the process that published it. A payload
// of another version reports errUnknownVersion; one that is not a known kind with its ID,
// errMalformedSignal.
func decodeSignal(payload []byte) (origin string, e Event, err error) {
	var s signal
	if err := json.Unmarshal(payload, &s); err != nil {
		return "", Event{}, errors.Join(errMalformedSignal, err)
	}
	if s.Version != signalVersion {
		return "", Event{}, fmt.Errorf("%w: %d", errUnknownVersion, s.Version)
	}
	if s.Kind.IsVoice() {
		if e, ok := voiceEvent(s); ok {
			return s.Origin, e, nil
		}
		return "", Event{}, fmt.Errorf("%w: kind %q", errMalformedSignal, s.Kind)
	}
	switch {
	case s.Kind == KindHomeEntry && s.E != "":
		return s.Origin, Event{Kind: KindHomeEntry, EntryID: s.E, AuthorAccountID: s.Author}, nil
	case s.Kind == KindNotifications && s.AccountID != "":
		return s.Origin, Event{Kind: KindNotifications, AccountID: s.AccountID}, nil
	case s.Kind == KindConversation && s.AccountID != "" && s.E != "":
		return s.Origin, Event{Kind: KindConversation, AccountID: s.AccountID, ConversationID: s.E}, nil
	case s.Kind == KindTLSCertificate && s.Host != "" && s.CertificateID != "":
		return s.Origin, Event{Kind: KindTLSCertificate, Host: s.Host, CertificateID: s.CertificateID}, nil
	case s.Kind == KindAnnouncement && s.AnnouncementID != "":
		return s.Origin, Event{Kind: KindAnnouncement, AnnouncementID: s.AnnouncementID}, nil
	case s.Kind == KindStatusDeleted && s.E != "" && s.Author != "" && s.Visibility != "":
		return s.Origin, Event{Kind: KindStatusDeleted, EntryID: s.E, AuthorAccountID: s.Author, AuthorDomain: s.Domain, Visibility: s.Visibility}, nil
	case s.Kind == KindPublicEntry && s.E != "":
		e := Event{Kind: KindPublicEntry, EntryID: s.E}
		for _, t := range s.Timelines {
			switch t {
			case TimelineLocal:
				e.OnLocal = true
			case TimelineFederated:
				e.OnFederated = true
			}
		}
		if e.OnLocal || e.OnFederated {
			return s.Origin, e, nil
		}
	}
	return "", Event{}, fmt.Errorf("%w: kind %q", errMalformedSignal, s.Kind)
}

// voiceEvent returns the event of a voice signal that has its room and every field of its kind.
func voiceEvent(s signal) (Event, bool) {
	e := Event{Kind: s.Kind, RoomID: s.RoomID}
	if s.RoomID == "" {
		return Event{}, false
	}
	switch s.Kind {
	case KindVoiceRoom:
		return e, true
	case KindVoiceSignal:
		e.MessageID, e.To = s.MessageID, s.To
		return e, s.MessageID != "" && s.To != ""
	case KindVoiceMic:
		if s.ParticipantID == "" || s.On == nil {
			return Event{}, false
		}
		e.ParticipantID, e.On = s.ParticipantID, *s.On
		return e, true
	case KindVoiceReaction:
		e.ParticipantID, e.Emoji = s.ParticipantID, s.E
		return e, s.ParticipantID != "" && s.E != "" && len(s.E) <= MaxEmojiSize
	case KindVoiceMute:
		e.SessionID, e.By = s.SessionID, s.By
		return e, s.SessionID != "" && s.By != ""
	}
	return Event{}, false
}
