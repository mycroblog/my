// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// ErrFederationRestricted reports something that the federation mode (FR-413, ADR-0006) does not
// let through: a refused inbox request, a fetch that is not made, or an activity that is not
// sent.
var ErrFederationRestricted = errors.New("federation: the federation mode does not let this through")

// ModeSettings reads the federation mode, the instance setting federation_mode (ADR-0006). Every
// entry point reads it per request and per job, so that a change applies at once, and asks
// federation.Allows about the flow it handles.
type ModeSettings interface {
	FederationMode(ctx context.Context) (federation.Mode, error)
	// FederationModeTx is FederationMode inside tx.
	FederationModeTx(ctx context.Context, tx uow.Tx) (federation.Mode, error)
}

// FixedMode is the ModeSettings of one mode, whatever the settings say.
type FixedMode federation.Mode

// FederationMode implements ModeSettings.
func (m FixedMode) FederationMode(context.Context) (federation.Mode, error) {
	return federation.Mode(m), nil
}

// FederationModeTx implements ModeSettings.
func (m FixedMode) FederationModeTx(context.Context, uow.Tx) (federation.Mode, error) {
	return federation.Mode(m), nil
}

// currentMode returns the mode that s reads, federation.DefaultMode when s is nil.
func currentMode(ctx context.Context, s ModeSettings) (federation.Mode, error) {
	if s == nil {
		return federation.DefaultMode, nil
	}
	m, err := s.FederationMode(ctx)
	if err != nil {
		return "", fmt.Errorf("read federation mode: %w", err)
	}
	return m, nil
}

// modeAllows reports whether the mode that s reads lets flow through.
func modeAllows(ctx context.Context, s ModeSettings, flow federation.Flow) (bool, error) {
	m, err := currentMode(ctx, s)
	if err != nil {
		return false, err
	}
	return federation.Allows(m, flow), nil
}

// modeAllowsTx is modeAllows inside tx.
func modeAllowsTx(ctx context.Context, tx uow.Tx, s ModeSettings, flow federation.Flow) (bool, error) {
	if s == nil {
		return federation.Allows(federation.DefaultMode, flow), nil
	}
	m, err := s.FederationModeTx(ctx, tx)
	if err != nil {
		return false, fmt.Errorf("read federation mode: %w", err)
	}
	return federation.Allows(m, flow), nil
}

// allowsAny reports whether mode lets one of flows through.
func allowsAny(mode federation.Mode, flows ...federation.Flow) bool {
	for _, f := range flows {
		if federation.Allows(mode, f) {
			return true
		}
	}
	return false
}

// receives reports whether mode lets the received activity act through to its handler
// (federation.md §3.2, ADR-0006 入口ごとの振る舞い); host is this server's, which tells the local
// objects. The handlers of the activities that span several flows decide the rest with the mode
// of InboxActivity:
//
//   - Follow (follow_in) passes when the mode allows it or refuses it: the follow handler answers
//     a refused one with a Reject.
//   - Accept and Reject answer a Follow of a local account (follow_out), of the instance actor's
//     membership of a relay, or a QuoteRequest (publish); they pass when one of those flows does,
//     and their handlers check their own.
//   - Undo passes by the activity it takes back: Follow (follow_in), Block (protect), Accept
//     (follow_out), and the reactions (ingest); an Undo whose object is not embedded passes when
//     one of those does.
//   - Block, Flag, and the Update or Delete of an actor by itself: protect.
//   - Like and EmojiReact, which are about local posts: ingest and publish, since the reactions
//     to posts that the mode does not publish are not shown anywhere.
//   - Announce: ingest, and publish too when it boosts a local post.
//   - QuoteRequest, about a local post: publish; its handler refuses it with a Reject when the
//     mode does not take the quoting post in.
//   - Move: follow_out, which it moves to the new actor.
//   - Create, Update, Delete, Add, Remove, and every other type: ingest.
func receives(mode federation.Mode, act *federation.Activity, host string) bool {
	switch act.Type {
	case federation.ActivityFollow:
		return federation.Allows(mode, federation.FlowFollowIn) || federation.Refuses(mode, federation.FlowFollowIn)
	case federation.ActivityAccept, federation.ActivityReject:
		return allowsAny(mode, federation.FlowFollowOut, federation.FlowPublish, federation.FlowIngest)
	case federation.ActivityUndo:
		if doc := act.Object().Untrusted(); doc != nil && doc.Activity != nil {
			// A follower that takes back its Follow ends what a refusing mode would refuse anyway.
			flow := undoneFlow(doc.Activity.Type)
			return federation.Allows(mode, flow) || federation.Refuses(mode, flow)
		}
		return allowsAny(mode, federation.FlowFollowIn, federation.FlowProtect, federation.FlowFollowOut, federation.FlowIngest)
	case federation.ActivityBlock, federation.ActivityFlag:
		return federation.Allows(mode, federation.FlowProtect)
	case federation.ActivityUpdate, federation.ActivityDelete:
		if ownActorActivity(act) {
			return federation.Allows(mode, federation.FlowProtect)
		}
		return federation.Allows(mode, federation.FlowIngest)
	case federation.ActivityLike, federation.ActivityEmojiReact:
		return federation.Allows(mode, federation.FlowIngest) && federation.Allows(mode, federation.FlowPublish)
	case federation.ActivityAnnounce:
		if host != "" && act.Object().ID.Host() == host {
			return federation.Allows(mode, federation.FlowIngest) && federation.Allows(mode, federation.FlowPublish)
		}
		return federation.Allows(mode, federation.FlowIngest)
	case federation.ActivityQuoteRequest:
		return federation.Allows(mode, federation.FlowPublish)
	case federation.ActivityMove:
		return federation.Allows(mode, federation.FlowFollowOut)
	}
	return federation.Allows(mode, federation.FlowIngest)
}

// undoneFlow is the flow of an Undo of an activity of type t.
func undoneFlow(t federation.ActivityType) federation.Flow {
	switch t {
	case federation.ActivityFollow:
		return federation.FlowFollowIn
	case federation.ActivityBlock:
		return federation.FlowProtect
	case federation.ActivityAccept:
		return federation.FlowFollowOut
	}
	return federation.FlowIngest
}

// identifies fails with ErrFederationRestricted when the federation mode that the Fetcher of r
// reads does not fetch actors and what identifies servers (identify): WebFinger and NodeInfo.
func (r *RemoteActors) identifies(ctx context.Context) error {
	var modes ModeSettings
	if r.d.Fetcher != nil {
		modes = r.d.Fetcher.d.Modes
	}
	switch ok, err := modeAllows(ctx, modes, federation.FlowIdentify); {
	case err != nil:
		return err
	case !ok:
		return fmt.Errorf("%w: identify", ErrFederationRestricted)
	}
	return nil
}

// FollowerReader tells the remote accounts that follow local ones; *appgraph.Service implements
// it.
type FollowerReader interface {
	FollowsAnyone(ctx context.Context, accountID string) (bool, error)
}

// MediaGate tells the media proxy which remote media files the federation mode lets it fetch
// (ADR-0006, 外の取得): every file when the mode takes content in (ingest), only the avatars and
// headers of the remote accounts that follow local ones when it only identifies them (identify,
// outbound), and none when it is off. Files without an owner, the images of link cards, are not
// federation's and always pass. It implements appcontent.MediaFederation.
type MediaGate struct {
	// Modes reads the federation mode; nil federates both ways.
	Modes     ModeSettings
	Followers FollowerReader
}

// ProxiesMedia implements appcontent.MediaFederation.
func (g MediaGate) ProxiesMedia(ctx context.Context, ownerAccountID string, profileImage bool) (bool, error) {
	if ownerAccountID == "" {
		return true, nil
	}
	mode, err := currentMode(ctx, g.Modes)
	switch {
	case err != nil:
		return false, err
	case federation.Allows(mode, federation.FlowIngest):
		return true, nil
	case !federation.Allows(mode, federation.FlowIdentify) || !profileImage || g.Followers == nil:
		return false, nil
	}
	return g.Followers.FollowsAnyone(ctx, ownerAccountID)
}
