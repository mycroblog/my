// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/platform/config"
)

// Voice methods: the values of the voice setting (ADR-0101 §1).
const (
	VoiceOff     = "off"
	VoiceP2P     = "p2p"
	VoiceLiveKit = "livekit"
	VoiceSora    = "sora"
)

// Voice is how the voice chat runs (ADR-0101 §1): the method that connects the rooms and the
// operator's limits of the rooms. The composition root passes it on to the voice chat; with
// the method off, the voice chat API answers 404 (ADR-0100 §2).
type Voice struct {
	// Method is VoiceOff, VoiceP2P, VoiceLiveKit, or VoiceSora.
	Method string
	// P2PMaxParticipants bounds a room of VoiceP2P, and MaxParticipants a room of an SFU (0 for
	// no limit).
	P2PMaxParticipants int
	MaxParticipants    int
	// MaxRoomDuration is the longest a room stays open, and MaxOpenRooms how many rooms may be
	// open at once; 0 is no limit.
	MaxRoomDuration time.Duration
	MaxOpenRooms    int
}

// Enabled reports whether the voice chat runs.
func (v Voice) Enabled() bool { return v.Method != "" && v.Method != VoiceOff }

// RoomCapacity returns how many people a room holds with the method, 0 for no limit.
func (v Voice) RoomCapacity() int {
	if v.Method == VoiceP2P {
		return v.P2PMaxParticipants
	}
	return v.MaxParticipants
}

// newVoice returns the Voice of cfg and logs the voice settings that work but were likely not
// meant (config.VoiceWarnings).
func newVoice(cfg *config.Config, logger *slog.Logger) Voice {
	v := Voice{
		Method: cfg.Voice, P2PMaxParticipants: cfg.VoiceP2PMaxParticipants, MaxParticipants: cfg.VoiceMaxParticipants,
		MaxRoomDuration: cfg.VoiceMaxRoomDuration, MaxOpenRooms: cfg.VoiceMaxOpenRooms,
	}
	if v.Method == "" {
		v.Method = VoiceOff
	}
	for _, f := range config.VoiceWarnings(cfg) {
		logger.Warn("a voice chat setting is likely not what was meant",
			slog.String("env", f.Env()), slog.String("detail", f.Key+" "+f.Message))
	}
	return v
}
