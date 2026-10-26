// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_voice_sora

package platform

import (
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/sora"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSoraTransport returns the transport of voice = sora (ADR-0102). The adapter reaches only
// the Sora server that the operator configured, or Sora Cloud, and a self-hosted one may be on
// a private address, so its HTTP client is not the SSRF-guarded kernel.HTTPClient; a call has
// 5 seconds.
func (a *App) newSoraTransport(cfg *config.Config) (appvoice.Transport, error) {
	return sora.New(sora.Options{
		Mode: cfg.VoiceSoraMode, SignalingURLs: cfg.VoiceSoraSignalingURLs, APIURL: cfg.VoiceSoraAPIURL,
		APIHeaderName: cfg.VoiceSoraAPIHeaderName, APIHeaderValue: cfg.VoiceSoraAPIHeaderValue,
		WebhookUsername: cfg.VoiceSoraWebhookUsername, WebhookPassword: cfg.VoiceSoraWebhookPassword,
		CloudProjectID: cfg.VoiceSoraCloudProjectID, CloudAPIKey: cfg.VoiceSoraCloudAPIKey, JoinSecret: cfg.VoiceJoinSecret,
		HTTPClient: &http.Client{Timeout: 5 * time.Second}, Clock: a.Clock,
	})
}
