// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/p2p"
	"github.com/mycroblog/my/internal/adapter/voice/turncloudflare"
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/queue"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
	voicehttp "github.com/mycroblog/my/internal/interface/voice"
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

// setUpVoice builds the voice module (ADR-0100): the projectors of its projections, its
// subscriptions to the events that change who may be in a room, its jobs, the rooms over the
// transport of the voice setting (ADR-0101 §2), and the endpoints of the events of the media
// servers (ADR-0102 §4). The jobs run with the voice chat off too, so that
// voice.sweep ends the rooms left open (disabled).
func (a *App) setUpVoice(cfg *config.Config, repo appvoice.Repository) error {
	appvoice.RegisterProjectors(a.Projectors, repo)
	heartbeats := appvoice.NewHeartbeats(a.KV, repo)
	var transport appvoice.Transport
	switch a.Voice.Method {
	case VoiceOff:
	case VoiceP2P:
		turn, err := a.voiceTURN(cfg)
		if err != nil {
			return err
		}
		transport = p2p.New(p2p.Options{
			STUNURLs: cfg.VoiceSTUNURLs, TURN: turn, TTL: cfg.VoiceTURNCredentialTTL,
			RelayOnly: cfg.VoiceICETransportPolicy == "relay", Sessions: heartbeats, Clock: a.Clock, Meter: a.Meter,
		})
	case VoiceLiveKit:
		var err error
		if transport, err = a.newLiveKitTransport(cfg); err != nil {
			return fmt.Errorf("voice = livekit: %w", err)
		}
	case VoiceSora:
		var err error
		if transport, err = a.newSoraTransport(cfg); err != nil {
			return fmt.Errorf("voice = sora: %w", err)
		}
	default:
		return fmt.Errorf("voice = %s: this build has no transport for it; use p2p, livekit, sora, or off", a.Voice.Method)
	}
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	c := appvoice.Config{Capacity: a.Voice.RoomCapacity(), MaxOpenRooms: a.Voice.MaxOpenRooms, MaxRoomDuration: a.Voice.MaxRoomDuration}
	if transport != nil {
		c.Transport = voice.Transport(a.Voice.Method)
	}
	a.VoiceRooms = appvoice.NewService(appvoice.Deps{
		Repository: repo, UnitOfWork: a.UnitOfWork, Outbox: outbox, Clock: a.Clock, IDs: a.IDs, Heartbeats: heartbeats,
		RateLimiter: a.RateLimiter, Transport: transport, Members: voiceMembers{a}, Relations: voiceRelations{a}, Config: c, Logger: a.Logger,
	})
	a.VoiceRooms.RegisterEligibilityProjectors(a.Projectors)
	a.VoiceRooms.RegisterJobs(a.Handlers, a.Scheduler)
	// The endpoints of the media servers' events answer 404 unless the voice chat runs on
	// their method.
	voicehttp.New(voicehttp.Options{Webhooks: a.VoiceRooms, Problem: a.writeStatusProblem, Logger: a.Logger}).
		Register(surfaceRouter{a, surfaceVoice})
	return nil
}

// voiceTURN returns the TURN source of voice_turn (ADR-0101 §5), nil for none.
func (a *App) voiceTURN(cfg *config.Config) (p2p.TURN, error) {
	switch cfg.VoiceTURN {
	case "", "none":
		return nil, nil
	case "coturn":
		return p2p.NewCoturn(cfg.VoiceTURNURLs, cfg.VoiceTURNSecret, a.Clock)
	case turncloudflare.Name:
		return turncloudflare.New(turncloudflare.Options{
			KeyID: cfg.VoiceTURNCloudflareKeyID, APIToken: cfg.VoiceTURNCloudflareAPIToken, KV: a.KV, Clock: a.Clock,
		})
	}
	return nil, fmt.Errorf("voice_turn = %s: unknown TURN source", cfg.VoiceTURN)
}

// voiceMembers reads the accounts for the rooms from the account, moderation, identity, and
// instance modules (ADR-0100 §2, §4).
type voiceMembers struct{ a *App }

func (m voiceMembers) Member(ctx context.Context, accountID string) (appvoice.Member, error) {
	act, err := m.a.Accounts.FindByID(ctx, accountID)
	if errors.Is(err, account.ErrNotFound) {
		return appvoice.Member{}, nil
	}
	if err != nil {
		return appvoice.Member{}, err
	}
	if !act.Account.IsLocal() || act.Local == nil || act.IsInstanceActor() {
		return appvoice.Member{}, nil
	}
	// A suspension or a ban restricts the federation of the account, and a limit its
	// unsolicited reach (data-model §5.2).
	rs, err := m.a.AccountPolicy.Restrictions(ctx, accountID)
	if err != nil {
		return appvoice.Member{}, err
	}
	suspended, limited := slices.Contains(rs, moderation.RestrictFederation), slices.Contains(rs, moderation.RestrictUnsolicitedReach)
	mayStart, err := m.a.Identity.HasPermission(ctx, act.Local.UserID, identity.PermStartVoiceRooms)
	if err != nil {
		return appvoice.Member{}, err
	}
	return appvoice.Member{Local: true, Active: !act.Deleted() && !suspended, Limited: limited, MayStart: mayStart}, nil
}

func (m voiceMembers) EveryoneStarts(ctx context.Context) (bool, error) {
	s, err := m.a.Instance.Settings(ctx)
	if err != nil {
		return false, err
	}
	return s.VoiceRoomStarters() == instance.VoiceRoomStartersEveryone, nil
}

// voiceRelations reads the follows and blocks of the rooms from the graph module.
type voiceRelations struct{ a *App }

func (r voiceRelations) Follows(ctx context.Context, followerID, followeeID string) (bool, error) {
	rel, err := r.a.Graph.CandidateRelations(ctx, followerID, []string{followeeID})
	return rel[followeeID].Following, err
}

func (r voiceRelations) Blocked(ctx context.Context, a, b string) (bool, error) {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		ok, err := r.a.Graph.IsBlocking(ctx, pair[0], pair[1])
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func (r voiceRelations) Following(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	rel, err := r.a.Graph.CandidateRelations(ctx, accountID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rel))
	for id, c := range rel {
		out[id] = c.Following
	}
	return out, nil
}

// voiceAPI returns the rooms of the client API, nil while the voice chat is off (404).
func (a *App) voiceAPI() clientapi.VoiceRooms {
	if a.VoiceRooms == nil || !a.VoiceRooms.Enabled() {
		return nil
	}
	return a.VoiceRooms
}
