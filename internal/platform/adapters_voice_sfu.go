// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_voice_sfu

package platform

import (
	"context"
	"net/url"

	"github.com/mycroblog/my/internal/adapter/voice/p2p"
	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/internal/adapter/voice/sfu/node"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSFU returns the transport of voice = sfu (ADR-0101 §13), which places the rooms on the media
// nodes of the registry in the KeyValueStore, and the media node of this process with role = all
// or sfu, nil otherwise (ADR-0101 §10). Neither starts anything: Serve starts the node.
func (a *App) newSFU(cfg *config.Config, turn p2p.TURN) (appvoice.Transport, VoiceNode, error) {
	registry := sfu.NewRegistry(a.KV, a.Clock)
	transport := sfu.New(sfu.Options{
		Registry: registry, TURN: turn, TTL: cfg.VoiceTURNCredentialTTL, MaxParticipants: cfg.VoiceMaxParticipants,
		Clock: a.Clock, Meter: a.Meter,
	})
	if !runsVoiceNode(cfg.Role) {
		return transport, nil, nil
	}
	n := node.New(node.Options{
		ID: a.IDs.NewID(), URL: cfg.VoiceSFUURL(), Capacity: cfg.VoiceSFUNodeCapacity,
		UDPPort: cfg.VoiceSFUUDPPort, TCPPort: cfg.VoiceSFUTCPPort, PublicIP: cfg.VoiceSFUPublicIP,
		OriginPatterns: voiceOriginPatterns(cfg), Registry: registry, Rooms: voiceMediaRooms{a}, Streams: a.Streams,
		Meter: a.Meter, Logger: a.Logger, Reporter: a.ErrorReporter,
	})
	return transport, n, nil
}

// runsVoiceNode reports whether a process of role runs the media node of voice = sfu: all and
// sfu. The web and worker roles only place the rooms and give the grants.
func runsVoiceNode(role config.Role) bool { return role == config.RoleAll || role == config.RoleSFU }

// voiceMediaRooms reads the rooms for the media node through the voice module, which is built
// after the node's transport.
type voiceMediaRooms struct{ a *App }

func (r voiceMediaRooms) MediaRoom(ctx context.Context, roomID string) (appvoice.MediaRoom, error) {
	return r.a.VoiceRooms.MediaRoom(ctx, roomID)
}

// voiceOriginPatterns returns the hosts of the pages that open the WebSocket of a media node
// besides its own: web_origin's and host.
func voiceOriginPatterns(cfg *config.Config) []string {
	hosts := []string{cfg.Host}
	if u, err := url.Parse(cfg.WebOrigin); err == nil && u.Host != "" && u.Host != cfg.Host {
		hosts = append(hosts, u.Host)
	}
	return hosts
}
