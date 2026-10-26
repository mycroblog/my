// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_voice_livekit

package platform

import (
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/livekit"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/platform/config"
)

// newLiveKitTransport returns the transport of voice = livekit (ADR-0102). The adapter reaches
// only the LiveKit server that the operator configured, which may be on a private address, so
// its HTTP client is not the SSRF-guarded kernel.HTTPClient; a call has 5 seconds.
func (a *App) newLiveKitTransport(cfg *config.Config) (appvoice.Transport, error) {
	return livekit.New(livekit.Options{
		URL: cfg.VoiceLiveKitURL, APIURL: cfg.VoiceLiveKitAPIURL, APIKey: cfg.VoiceLiveKitAPIKey, APISecret: cfg.VoiceLiveKitAPISecret,
		MaxParticipants: cfg.VoiceMaxParticipants, HTTPClient: &http.Client{Timeout: 5 * time.Second}, Clock: a.Clock,
	})
}
