// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/i18n"
)

// Profiles changes the viewer's profile. *appaccount.Service implements it.
type Profiles interface {
	// UpdateProfile records a new profile revision; it fails with an error wrapping
	// account.ErrInvalidProfile for invalid content.
	UpdateProfile(ctx context.Context, accountID string, in appaccount.ProfileUpdate) (appaccount.Actor, error)
}

// PostingPolicy reads what an account's restrictions let it post. *appcontent.Service
// implements it.
type PostingPolicy interface {
	PostingLimits(ctx context.Context, accountID string) (appcontent.PostingLimits, error)
}

// newCredentialAccount presents the viewer's own actor as the CredentialAccount schema: the
// account with its follow counts and images (newProfile), the source of its profile, its
// posting limits, and the permissions of the user's roles.
func (s *Server) newCredentialAccount(ctx context.Context, userID string, a appaccount.Actor) (CredentialAccount, error) {
	base, err := s.newProfile(ctx, a)
	if err != nil {
		return CredentialAccount{}, err
	}
	posting, err := s.postingLimits(ctx, a.Account.ID)
	if err != nil {
		return CredentialAccount{}, err
	}
	permissions, err := s.permissions(ctx, userID)
	if err != nil {
		return CredentialAccount{}, err
	}
	out := CredentialAccount{
		Id: base.Id, Username: base.Username, Acct: base.Acct, DisplayName: base.DisplayName, Note: base.Note,
		Url: base.Url, Locked: base.Locked, Bot: base.Bot, CreatedAt: base.CreatedAt, Fields: base.Fields, Emojis: base.Emojis,
		Badges: base.Badges,
		Avatar: base.Avatar, Header: base.Header,
		FollowersCount: base.FollowersCount, FollowingCount: base.FollowingCount,
		Source: AccountSource{Fields: []AccountFieldSource{}}, Posting: posting, Permissions: permissions,
	}
	p := a.Profile
	if p == nil {
		return out, nil
	}
	out.Discoverable, out.Indexable, out.AiOptOut = p.Discoverable, p.Indexable, p.AIOptOut
	for i, f := range p.Fields {
		src := AccountFieldSource{Name: f.Name}
		if p.Source != nil && i < len(p.Source.FieldValues) {
			src.Value = p.Source.FieldValues[i]
		}
		out.Source.Fields = append(out.Source.Fields, src)
	}
	if p.Source != nil {
		out.Source.Note = p.Source.Note
	}
	if p.AvatarMediaID != "" {
		out.Source.AvatarMediaId = &p.AvatarMediaID
	}
	if p.HeaderMediaID != "" {
		out.Source.HeaderMediaId = &p.HeaderMediaID
	}
	return out, nil
}

// postingLimits presents the posting limits of accountID; without Options.Posting the account
// is unrestricted.
func (s *Server) postingLimits(ctx context.Context, accountID string) (PostingLimits, error) {
	none := content.AuthorRestrictions{}
	l := appcontent.PostingLimits{VisibilityCeiling: none.VisibilityCeiling(), Visibilities: content.PostingVisibilities(none)}
	if s.o.Posting != nil {
		var err error
		if l, err = s.o.Posting.PostingLimits(ctx, accountID); err != nil {
			return PostingLimits{}, fmt.Errorf("clientapi: posting limits of %s: %w", accountID, err)
		}
	}
	out := PostingLimits{VisibilityCeiling: Visibility(l.VisibilityCeiling), Visibilities: make([]Visibility, 0, len(l.Visibilities))}
	for _, v := range l.Visibilities {
		out.Visibilities = append(out.Visibilities, Visibility(v))
	}
	return out, nil
}

// UpdateAccountCredentials serves PATCH /api/v1/accounts/update_credentials: a new revision of
// the viewer's profile (FR-006).
func (s *Server) UpdateAccountCredentials(ctx context.Context, req UpdateAccountCredentialsRequestObject) (UpdateAccountCredentialsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	in := appaccount.ProfileUpdate{}
	if b := req.Body; b != nil {
		in = appaccount.ProfileUpdate{
			DisplayName: b.DisplayName, Note: b.Note, AvatarMediaID: knownIDPtr(b.AvatarMediaId), HeaderMediaID: knownIDPtr(b.HeaderMediaId),
			Locked: b.Locked, Bot: b.Bot, Discoverable: b.Discoverable, Indexable: b.Indexable, AIOptOut: b.AiOptOut,
		}
		if b.Fields != nil {
			fields := make([]account.FieldSource, len(*b.Fields))
			for i, f := range *b.Fields {
				fields[i] = account.FieldSource{Name: f.Name, Value: f.Value}
			}
			in.Fields = &fields
		}
	}
	a, err := s.o.Profiles.UpdateProfile(ctx, v.AccountID, in)
	var invalid *account.ProfileError
	if errors.As(err, &invalid) {
		body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank,
			s.Localizer(ctx).ServerErrorsUnprocessable(), s.profileProblemDetail(ctx, invalid))
		return UpdateAccountCredentialsdefaultApplicationProblemPlusJSONResponse{
			Body: body, Headers: headers, StatusCode: http.StatusUnprocessableEntity,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update the profile of %s: %w", v.AccountID, err)
	}
	body, err := s.newCredentialAccount(ctx, v.UserID, a)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return UpdateAccountCredentials200JSONResponse{
		Body:    body,
		Headers: UpdateAccountCredentials200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// profileProperties are the request properties of the parts of a profile.
var profileProperties = map[account.ProfileProperty]string{
	account.PropertyDisplayName: "display_name",
	account.PropertyNote:        "note",
	account.PropertyFields:      "fields",
	account.PropertyFieldName:   "fields[].name",
	account.PropertyFieldValue:  "fields[].value",
	account.PropertyAvatar:      "avatar_media_id",
	account.PropertyHeader:      "header_media_id",
}

func (s *Server) profileProblemDetail(ctx context.Context, e *account.ProfileError) string {
	l := s.Localizer(ctx)
	name := profileProperties[e.Property]
	switch {
	case e.Problem == account.ProblemTooLong:
		return l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: e.Max, Name: name})
	case e.Problem == account.ProblemTooMany:
		return l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: e.Max, Name: name})
	case e.Property == account.PropertyAvatar || e.Property == account.PropertyHeader:
		return l.ServerErrorsUnusableImage(name)
	}
	return l.ServerErrorsInvalidValue(name)
}
