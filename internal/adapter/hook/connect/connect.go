// SPDX-License-Identifier: AGPL-3.0-only

// Package connect is the adapter of the hook Plugin port over the plugin protocol
// (api/plugin/my/plugin/hook/v1, ADR-0146, ADR-0147).
package connect

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"connectrpc.com/connect/v2"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	hookv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1/hookv1connect"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/domain/hook"
)

// Plugin implements apphook.Plugin on one hook's connection.
type Plugin struct {
	client hookv1connect.HookServiceClient
}

var _ apphook.Plugin = (*Plugin)(nil)

// New returns the plugin behind conn, the connection of one hook's kind.
func New(conn *client.Conn) *Plugin {
	return &Plugin{client: hookv1connect.NewHookServiceClient(conn.RPC)}
}

// Check implements apphook.Plugin. A transport failure reaches the caller as a kind of failure with
// no text of the plugin's, and an answer outside the vocabulary as apphook.ErrPluginInvalid.
func (p *Plugin) Check(ctx context.Context, r apphook.Request) (apphook.Answer, error) {
	req := &hookv1.CheckRequest{Hook: string(r.Hook), SubjectKey: r.SubjectKey}
	if err := setSubject(req, r.Subject); err != nil {
		return apphook.Answer{}, err
	}
	res, err := p.client.Check(ctx, req)
	if err != nil {
		return apphook.Answer{}, callError(err)
	}
	verdict, ok := verdictOf(res.GetVerdict())
	if !ok {
		return apphook.Answer{}, fmt.Errorf("%w: verdict %d", apphook.ErrPluginInvalid, int32(res.GetVerdict()))
	}
	return apphook.Answer{Verdict: verdict, Reason: reasonOf(res.GetReason()), Note: res.GetNote()}, nil
}

// setSubject writes the description of the subject that a hook sends. Each concrete hook adds a
// case for its type and a field of CheckRequest.
func setSubject(req *hookv1.CheckRequest, subject any) error {
	switch s := subject.(type) {
	case nil:
		return nil
	case apphook.InboundStatusSubject:
		req.InboundStatus = inboundStatus(s)
		return nil
	case apphook.FollowSubject:
		req.Follow = &hookv1.FollowSubject{FollowerUri: s.FollowerURI, FolloweeUri: s.FolloweeURI}
		return nil
	case apphook.SignupSubject:
		req.Signup = &hookv1.SignupSubject{Email: s.Email, Ip: s.IP}
		return nil
	case apphook.LocalStatusSubject:
		req.LocalStatus = &hookv1.LocalStatus{
			StatusUri: s.StatusURI, AuthorUri: s.AuthorURI, AuthorUsername: s.AuthorUsername,
			AccountCreatedAt: s.AccountCreatedAt.UTC().Format(time.RFC3339Nano), Visibility: s.Visibility,
			InReplyToUri: s.InReplyToURI, QuoteUri: s.QuoteURI, Language: s.Language, SpoilerText: s.SpoilerText,
			Sensitive: s.Sensitive, Text: s.Text, Html: s.HTML, AttachmentDescriptions: s.AttachmentDescriptions,
			PollOptions: s.PollOptions, Hashtags: s.Hashtags, MentionCount: count32(s.MentionCount),
		}
		return nil
	case apphook.MediaSubject:
		req.Media = mediaSubject(s)
		return nil
	case apphook.ActorSubject:
		req.InboundActor = inboundActor(s)
		return nil
	case apphook.LocalProfileSubject:
		req.LocalProfile = localProfile(s)
		return nil
	}
	return fmt.Errorf("hook: no mapping of the subject %T", subject)
}

func inboundStatus(s apphook.InboundStatusSubject) *hookv1.InboundStatus {
	out := &hookv1.InboundStatus{
		ActivityType: s.ActivityType, StatusUri: s.StatusURI, StatusUrl: s.StatusURL, AuthorUri: s.AuthorURI, AuthorDomain: s.AuthorDomain,
		Visibility: s.Visibility, AnyLocalFollower: s.AnyLocalFollower, InReplyToUri: s.InReplyToURI, SpoilerText: s.SpoilerText,
		Sensitive: s.Sensitive, PollOptions: s.PollOptions, Hashtags: s.Hashtags,
		MentionCount: count32(s.MentionCount), LocalMentionCount: count32(s.LocalMentionCount),
	}
	if !s.PublishedAt.IsZero() {
		out.PublishedAt = s.PublishedAt.UTC().Format(time.RFC3339)
	}
	for _, b := range s.Bodies {
		out.Bodies = append(out.Bodies, &hookv1.InboundStatus_Body{Language: b.Language, Html: b.HTML})
	}
	for _, a := range s.Attachments {
		out.Attachments = append(out.Attachments, &hookv1.InboundStatus_Attachment{Type: a.Type, Url: a.URL, ContentType: a.ContentType, Description: a.Description})
	}
	return out
}

func inboundActor(s apphook.ActorSubject) *hookv1.InboundActor {
	out := &hookv1.InboundActor{
		ActorUri: s.ActorURI, Domain: s.Domain, Username: s.Username, NewActor: s.New,
		DisplayName: s.DisplayName, NoteHtml: s.NoteHTML, AvatarUrl: s.AvatarURL, HeaderUrl: s.HeaderURL,
	}
	for _, f := range s.Fields {
		out.Fields = append(out.Fields, &hookv1.InboundActor_Field{Name: f.Name, ValueHtml: f.ValueHTML})
	}
	return out
}

func mediaSubject(s apphook.MediaSubject) *hookv1.MediaSubject {
	out := &hookv1.MediaSubject{
		Purpose: s.Purpose, MediaType: s.MediaType, Width: count32(s.Width), Height: count32(s.Height),
		DurationMs: s.Duration.Milliseconds(), Animated: s.Animated,
	}
	if len(s.Image) > 0 {
		out.Image, out.ImageContentType = s.Image, s.ImageContentType
		out.ImageWidth, out.ImageHeight = count32(s.ImageWidth), count32(s.ImageHeight)
	}
	return out
}

func localProfile(s apphook.LocalProfileSubject) *hookv1.LocalProfile {
	out := &hookv1.LocalProfile{
		AccountUri: s.AccountURI, Username: s.Username, AccountCreatedAt: s.AccountCreatedAt.UTC().Format(time.RFC3339Nano),
		DisplayName: s.DisplayName, Note: s.Note, NoteHtml: s.NoteHTML,
		AvatarChanged: s.AvatarChanged, HeaderChanged: s.HeaderChanged,
	}
	for _, f := range s.Fields {
		out.Fields = append(out.Fields, &hookv1.LocalProfile_Field{Name: f.Name, Value: f.Value, ValueHtml: f.ValueHTML})
	}
	return out
}

// count32 is a count of what one post names, which federation.MaxTags keeps far below the limit,
// or a side of an image, which the media limits keep there.
func count32(n int) int32 { return int32(min(n, math.MaxInt32)) } //nolint:gosec // G115: min bounds it

func callError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || connect.CodeOf(err) == connect.CodeDeadlineExceeded {
		return fmt.Errorf("%w: %s", apphook.ErrPluginTimeout, connect.CodeOf(err))
	}
	return fmt.Errorf("%w: %s", apphook.ErrPluginFailed, connect.CodeOf(err))
}

func verdictOf(v hookv1.Verdict) (hook.Outcome, bool) {
	switch v {
	case hookv1.Verdict_VERDICT_ALLOW:
		return hook.Allow, true
	case hookv1.Verdict_VERDICT_DROP:
		return hook.Drop, true
	case hookv1.Verdict_VERDICT_FLAG:
		return hook.Flag, true
	case hookv1.Verdict_VERDICT_MARK_SENSITIVE:
		return hook.MarkSensitive, true
	case hookv1.Verdict_VERDICT_REQUIRE_APPROVAL:
		return hook.RequireApproval, true
	case hookv1.Verdict_VERDICT_REJECT:
		return hook.Reject, true
	}
	return "", false
}

func reasonOf(r hookv1.Reason) hook.Reason {
	switch r {
	case hookv1.Reason_REASON_SPAM:
		return hook.ReasonSpam
	case hookv1.Reason_REASON_HARASSMENT:
		return hook.ReasonHarassment
	case hookv1.Reason_REASON_SEXUAL:
		return hook.ReasonSexual
	case hookv1.Reason_REASON_VIOLENCE:
		return hook.ReasonViolence
	case hookv1.Reason_REASON_ILLEGAL:
		return hook.ReasonIllegal
	case hookv1.Reason_REASON_MALWARE:
		return hook.ReasonMalware
	case hookv1.Reason_REASON_OTHER:
		return hook.ReasonOther
	}
	return hook.ReasonNone
}
