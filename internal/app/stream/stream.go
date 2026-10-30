// SPDX-License-Identifier: AGPL-3.0-only

// Package stream defines the ports of the real-time updates that the streaming endpoint serves
// (FR-207, ADR-0016): modules publish signals after their transactions commit, and each open
// stream subscribes to the signals of its account. It also holds Hub, the in-process broker
// that implements both ports for one process, and Relay, which implements them for every
// process on a Bus, the port of the cross-process brokers (ADR-0050).
//
// A signal names what changed, not what a viewer sees: the subscriber reads the change back
// through the use cases of the module that owns it, which apply the visibility policy for that
// viewer. Delivery is best effort: a signal published while no stream is open, or to a stream
// that does not keep up, is lost, and clients catch up by reading the timelines and
// notifications again when they reconnect.
package stream

import (
	"context"
	"errors"
)

// Kind is the kind of a signal.
type Kind string

const (
	// KindHomeEntry signals a new entry of the home timelines (a post or a boost). It goes to
	// every subscriber, which decides whether its own home timeline lists the entry.
	KindHomeEntry Kind = "home_entry"
	// KindNotifications signals that an account may have new notifications.
	KindNotifications Kind = "notifications"
	// KindAnnouncement signals that a server-wide announcement was published, changed while
	// published, or withdrawn (FR-726). It goes to every subscriber, which reads whether its
	// viewer sees the announcement now.
	KindAnnouncement Kind = "announcement"
	// KindPublicEntry signals a new post that the public timelines may list (ADR-0127 §4): it
	// goes to the timeline subscriptions of the timelines it names (OnLocal, OnFederated), each
	// of which decides whether its viewer's timeline lists the post.
	KindPublicEntry Kind = "public_entry"

	// The signals of a voice chat room (ADR-0101 §3) go to the room subscriptions of their
	// RoomID only. KindVoiceRoom signals that the room or its participants changed: each
	// subscriber reads the room again.
	KindVoiceRoom Kind = "voice_room"
	// KindVoiceSignal carries MessageID, a message for the session To of the room that waits
	// in the KeyValueStore of the voice module: the subscriber of that session reads it.
	KindVoiceSignal Kind = "voice_signal"
	// KindVoiceMic carries the microphone of ParticipantID turned on or off (On).
	KindVoiceMic Kind = "voice_mic"
	// KindVoiceReaction carries the Emoji that ParticipantID reacted with.
	KindVoiceReaction Kind = "voice_reaction"
	// KindVoiceMute carries the session SessionID muted by the account By.
	KindVoiceMute Kind = "voice_mute"
)

// Timeline names a public timeline whose new posts a timeline subscription receives (ADR-0127).
type Timeline string

const (
	// TimelineLocal is the local timeline: the posts of the local accounts.
	TimelineLocal Timeline = "local"
	// TimelineFederated is the federated timeline: the public posts of local and remote accounts.
	TimelineFederated Timeline = "federated"
)

// Valid reports whether t names a public timeline.
func (t Timeline) Valid() bool { return t == TimelineLocal || t == TimelineFederated }

// MaxEmojiSize is the largest Emoji of a KindVoiceReaction signal, in bytes: a Unicode emoji or
// a :shortcode: (ADR-0101 §3).
const MaxEmojiSize = 64

// IsVoice reports whether k is a kind of the voice chat rooms, which goes to the subscriptions
// of its room only.
func (k Kind) IsVoice() bool {
	switch k {
	case KindVoiceRoom, KindVoiceSignal, KindVoiceMic, KindVoiceReaction, KindVoiceMute:
		return true
	}
	return false
}

// Event is one signal.
type Event struct {
	Kind Kind
	// AccountID is the account whose notifications changed (KindNotifications).
	AccountID string
	// EntryID is the ID of the new timeline entry (KindHomeEntry, KindPublicEntry): a status ID
	// for a post, a boost ID for a boost.
	EntryID string
	// OnLocal and OnFederated name the public timelines that may list the post of a
	// KindPublicEntry signal; at least one is set.
	OnLocal     bool
	OnFederated bool
	// AnnouncementID is the ID of the announcement that changed (KindAnnouncement).
	AnnouncementID string

	// RoomID is the voice chat room of a voice signal.
	RoomID string
	// MessageID names the message of KindVoiceSignal, and To the session it is for.
	MessageID string
	To        string
	// ParticipantID is the account of the participant of KindVoiceMic and KindVoiceReaction.
	ParticipantID string
	// On is the microphone of KindVoiceMic.
	On bool
	// Emoji is the reaction of KindVoiceReaction, at most MaxEmojiSize bytes.
	Emoji string
	// SessionID is the muted session of KindVoiceMute, and By the account that muted it.
	SessionID string
	By        string
}

// Publisher publishes signals. Publish never blocks on subscribers or brokers and never fails
// the caller, so commit hooks can call it (uow.CommitHook).
type Publisher interface {
	Publish(e Event)
}

// Errors of Subscriber.Subscribe.
var (
	// ErrTooManySubscriptions reports that the account, or the process, has as many open
	// subscriptions as it allows.
	ErrTooManySubscriptions = errors.New("stream: too many open subscriptions")
	// ErrClosed reports a broker that is shutting down.
	ErrClosed = errors.New("stream: the broker is closed")
	// ErrUnavailable reports a broker that cannot receive the signals of other processes right
	// now: its listener failed to start or was not ready in time. A later attempt may succeed.
	ErrUnavailable = errors.New("stream: the broker is unavailable")
)

// Subscriber opens subscriptions.
type Subscriber interface {
	// Subscribe opens a subscription to the signals of accountID: its notifications and every
	// home timeline entry. Close it when done. It may wait until the broker is ready to receive
	// the signals of every process, for as long as ctx allows and at most a bound of the
	// broker's own; a broker that is not ready by then reports ErrUnavailable.
	Subscribe(ctx context.Context, accountID string) (Subscription, error)
}

// RoomSubscriber opens the subscriptions of the voice chat rooms (ADR-0101 §3).
type RoomSubscriber interface {
	// SubscribeRoom opens a subscription of the session session to the signals of the voice
	// chat room roomID: every signal of the room but the ones for another session
	// (KindVoiceSignal to another To, KindVoiceMute of another SessionID). It counts toward the
	// subscriptions of the process, not of an account, and waits for the broker as Subscribe
	// does.
	SubscribeRoom(ctx context.Context, roomID, session string) (Subscription, error)
}

// TimelineSubscriber opens the subscriptions of the public timeline streams (ADR-0127).
type TimelineSubscriber interface {
	// SubscribeTimeline opens a subscription of accountID to the KindPublicEntry signals of the
	// timeline t. It counts toward the subscriptions of the account, as Subscribe does, and
	// waits for the broker as Subscribe does.
	SubscribeTimeline(ctx context.Context, accountID string, t Timeline) (Subscription, error)
}

// Subscription is one open subscription.
type Subscription interface {
	// Events delivers the signals in the order they were published in each process; signals
	// of different processes may interleave in any order, and a cross-process broker may
	// deliver one twice.
	Events() <-chan Event
	// Done is closed when the subscription ends: Close was called, the subscriber fell behind
	// (its buffer filled up), the broker may have missed signals (the listener of a Relay
	// broke), or the broker closed.
	Done() <-chan struct{}
	// Close ends the subscription and releases it. Calling it again does nothing.
	Close()
}
