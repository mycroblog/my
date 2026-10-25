// SPDX-License-Identifier: MIT

package mybottest

import (
	"strings"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// account is an account of acct ("alice" local, "alice@remote.example" remote), with every field that the spec
// requires.
func account(id, acct string, createdAt time.Time) mykit.Account {
	username, domain, remote := strings.Cut(acct, "@")
	if !remote {
		domain = Domain
	}
	return normalizeAccount(mykit.Account{
		Id:        id,
		Acct:      acct,
		Username:  username,
		Domain:    domain,
		CreatedAt: createdAt,
		Uri:       "https://" + domain + "/users/" + username,
		Url:       "https://" + domain + "/@" + username,
	})
}

// credentialAccount is the bot's own account username, with the bot flag.
func credentialAccount(id, username string, createdAt time.Time) mykit.CredentialAccount {
	return mykit.CredentialAccount{
		Id:          id,
		Acct:        username,
		Username:    username,
		Domain:      Domain,
		Bot:         true,
		CreatedAt:   createdAt,
		Uri:         "https://" + Domain + "/users/" + username,
		Url:         "https://" + Domain + "/@" + username,
		Badges:      []mykit.AccountBadge{},
		Emojis:      []mykit.CustomEmoji{},
		Fields:      []mykit.ProfileField{},
		Permissions: []mykit.Permission{},
		Posting: mykit.PostingLimits{
			Visibilities: []mykit.Visibility{
				mykit.VisibilityPublic, mykit.VisibilityLocal, mykit.VisibilityUnlisted, mykit.VisibilityPrivate, mykit.VisibilityDirect,
			},
			VisibilityCeiling: mykit.VisibilityPublic,
		},
		Source: mykit.AccountSource{Fields: []mykit.AccountFieldSource{}},
	}
}

// normalizeAccount fills in what the spec requires and a test may leave out: the lists, which must not be null, and
// the addresses.
func normalizeAccount(a mykit.Account) mykit.Account {
	if a.Badges == nil {
		a.Badges = []mykit.AccountBadge{}
	}
	if a.Emojis == nil {
		a.Emojis = []mykit.CustomEmoji{}
	}
	if a.Fields == nil {
		a.Fields = []mykit.ProfileField{}
	}
	if a.Username == "" {
		a.Username, _, _ = strings.Cut(a.Acct, "@")
	}
	if a.Domain == "" {
		if _, d, ok := strings.Cut(a.Acct, "@"); ok {
			a.Domain = d
		} else {
			a.Domain = Domain
		}
	}
	if a.Uri == "" {
		a.Uri = "https://" + a.Domain + "/users/" + a.Username
	}
	if a.Url == "" {
		a.Url = "https://" + a.Domain + "/@" + a.Username
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Unix(0, 0).UTC()
	}
	return a
}

// normalizeStatus fills in what the spec requires of a post and a test may leave out.
func normalizeStatus(st mykit.Status) mykit.Status {
	st.Account = normalizeAccount(st.Account)
	if st.Emojis == nil {
		st.Emojis = []mykit.CustomEmoji{}
	}
	if st.MediaAttachments == nil {
		st.MediaAttachments = []mykit.MediaAttachment{}
	}
	if st.Mentions == nil {
		st.Mentions = []mykit.StatusMention{}
	}
	for i, m := range st.Mentions {
		if m.Username == "" {
			st.Mentions[i].Username, _, _ = strings.Cut(m.Acct, "@")
		}
		if m.Url == "" {
			st.Mentions[i].Url = "https://" + Domain + "/@" + st.Mentions[i].Username
		}
	}
	if st.Reactions == nil {
		st.Reactions = []mykit.StatusReaction{}
	}
	if st.Tags == nil {
		st.Tags = []mykit.StatusTag{}
	}
	if st.Language == "" {
		st.Language = "und"
	}
	if st.Visibility == "" {
		st.Visibility = mykit.VisibilityPublic
	}
	if st.Uri == "" {
		st.Uri = "https://" + st.Account.Domain + "/statuses/" + st.Id
	}
	if st.CreatedAt.IsZero() {
		st.CreatedAt = time.Unix(0, 0).UTC()
	}
	if st.Boost != nil {
		b := normalizeStatus(*st.Boost)
		st.Boost = &b
	}
	return st
}

// normalizeNotification fills in what the spec requires of a notification and a test may leave out.
func normalizeNotification(n mykit.Notification) mykit.Notification {
	if n.Account != nil {
		a := normalizeAccount(*n.Account)
		n.Account = &a
	}
	if n.Status != nil {
		st := normalizeStatus(*n.Status)
		n.Status = &st
	}
	return n
}
