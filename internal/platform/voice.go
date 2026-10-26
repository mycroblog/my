// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
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
	"github.com/mycroblog/my/internal/interface/web"
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
		RateLimiter: a.RateLimiter, Transport: transport, Members: voiceMembers{a}, Relations: voiceRelations{a},
		Streams: a.Streams, Emojis: a.Interactions, Notifier: a.voiceNotifier, Posts: a.Content, Host: cfg.Host,
		Config: c, Logger: a.Logger,
	})
	a.VoiceRooms.RegisterCommitHooks(a.Projectors)
	a.VoiceRooms.RegisterEligibilityProjectors(a.Projectors)
	a.VoiceRooms.RegisterJobs(a.Handlers, a.Scheduler)
	// The endpoints of the media servers' events answer 404 unless the voice chat runs on
	// their method.
	voicehttp.New(voicehttp.Options{Webhooks: a.VoiceRooms, Problem: a.writeStatusProblem, Logger: a.Logger}).
		Register(surfaceRouter{a, surfaceVoice})
	return nil
}

// voiceHTTPClient returns the HTTP client of a voice adapter that calls a server the operator
// configured (an SFU, Cloudflare's TURN service; ADR-0102 §1): it does not follow redirects, so
// that a request and its credentials reach that server only.
func voiceHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
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
			HTTPClient: voiceHTTPClient(10 * time.Second),
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

func (m voiceMembers) PublicSummary(ctx context.Context) (bool, error) {
	s, err := m.a.Instance.Settings(ctx)
	if err != nil {
		return false, err
	}
	return s.VoicePublicSummary(), nil
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

func (r voiceRelations) BlockedAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	return r.a.Graph.BlockedAmong(ctx, accountID, ids)
}

func (r voiceRelations) LocalFollowers(ctx context.Context, accountID, before string, limit int) ([]string, string, error) {
	ids, next, err := r.a.Graph.ActiveFollowerIDs(ctx, accountID, before, limit)
	if err != nil {
		return nil, "", err
	}
	local := make([]string, 0, len(ids))
	for _, id := range ids {
		act, err := r.a.Accounts.FindByID(ctx, id)
		if errors.Is(err, account.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		if act.Account.IsLocal() && act.Local != nil {
			local = append(local, id)
		}
	}
	return local, next, nil
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

// voiceRoomSummary returns the summary of a room for the link preview of its page, ok false
// when the room shows none, is unknown, or the voice chat is off (ADR-0100 §9).
func (a *App) voiceRoomSummary(ctx context.Context, roomID string) (web.VoiceRoomSummary, bool, error) {
	if a.voiceAPI() == nil {
		return web.VoiceRoomSummary{}, false, nil
	}
	s, err := a.VoiceRooms.Summary(ctx, roomID)
	if errors.Is(err, appvoice.ErrNotFound) {
		return web.VoiceRoomSummary{}, false, nil
	}
	if err != nil || !s.Shown {
		return web.VoiceRoomSummary{}, false, err
	}
	out := web.VoiceRoomSummary{Name: s.Name, Description: s.Description, Ended: s.Ended}
	if s.HostAccountID != "" {
		act, err := a.Accounts.FindByID(ctx, s.HostAccountID)
		switch {
		case errors.Is(err, account.ErrNotFound):
		case err != nil:
			return web.VoiceRoomSummary{}, false, err
		default:
			if act.Local != nil {
				out.HostName = act.Local.Username
			}
			if act.Profile != nil && strings.TrimSpace(act.Profile.DisplayName) != "" {
				out.HostName = act.Profile.DisplayName
			}
		}
	}
	return out, true, nil
}

// voiceAPI returns the rooms of the client API, nil while the voice chat is off (404).
func (a *App) voiceAPI() clientapi.VoiceRooms {
	if a.VoiceRooms == nil || !a.VoiceRooms.Enabled() {
		return nil
	}
	return a.VoiceRooms
}
