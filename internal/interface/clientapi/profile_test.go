// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const avatarID = "01890a5d-ac96-774b-bcce-b302099a80aa"

// fakeProfiles applies a profile update to a copy of an actor, or fails with err.
type fakeProfiles struct {
	actor appaccount.Actor
	got   []appaccount.ProfileUpdate
	err   error
}

func (f *fakeProfiles) UpdateProfile(_ context.Context, _ string, in appaccount.ProfileUpdate) (appaccount.Actor, error) {
	f.got = append(f.got, in)
	if f.err != nil {
		return appaccount.Actor{}, f.err
	}
	a := f.actor
	p := *a.Profile
	p.Seq++
	if in.DisplayName != nil {
		p.DisplayName = *in.DisplayName
	}
	if in.Note != nil {
		p.NoteHTML = "<p>" + *in.Note + "</p>"
		p.Source = &account.ProfileSource{Note: *in.Note}
	}
	if in.AvatarMediaID != nil {
		p.AvatarMediaID = *in.AvatarMediaID
	}
	if in.Fields != nil {
		p.Fields, p.Source.FieldValues = nil, nil
		for i, fs := range *in.Fields {
			p.Fields = append(p.Fields, account.ProfileField{Position: i, Name: fs.Name, ValueHTML: "<p>" + fs.Value + "</p>"})
			p.Source.FieldValues = append(p.Source.FieldValues, fs.Value)
		}
	}
	a.Profile = &p
	f.actor = a
	return a, nil
}

// profileMedia returns a fakeMedia holding avatarID, a processed image that alice uploaded.
func profileMedia() *fakeMedia {
	f := newFakeMedia()
	f.files[avatarID] = appcontent.Media{File: content.MediaFile{ID: avatarID, OwnerAccountID: aliceAccountID, Origin: content.MediaOriginUpload}}
	f.process(avatarID, content.MediaReady)
	return f
}

// sendJSON sends a request with a JSON body, which must conform to the spec unless it contains
// "invalid-on-purpose".
func (e *authEnv) sendJSON(t *testing.T, method, path, token, body string) *http.Response {
	t.Helper()
	newRequest := func() *http.Request {
		req := httptest.NewRequest(method, clientapi.BasePath+path, strings.NewReader(body))
		req.RemoteAddr = "192.0.2.1:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", "en")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return req
	}
	if !strings.Contains(body, "invalid-on-purpose") {
		e.spec.ValidateRequest(t, newRequest())
	}
	return e.spec.Serve(t, e.h, newRequest())
}

func aliceProfile() *account.ProfileRevision {
	return &account.ProfileRevision{
		Seq: 3, DisplayName: "Alice", NoteHTML: "<p>hi</p>", Locked: true, Discoverable: true,
		AvatarMediaID: avatarID,
		Fields:        []account.ProfileField{{Name: "web", ValueHTML: `<p><a href="https://alice.example">alice.example</a></p>`}},
		Source:        &account.ProfileSource{Note: "hi", FieldValues: []string{"https://alice.example"}},
	}
}

func TestVerifyAccountCredentialsHasTheSource(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		alice := o.Accounts.(fakeAccounts)[aliceAccountID]
		alice.Profile = aliceProfile()
		o.Accounts = fakeAccounts{aliceAccountID: alice}
		o.Media = profileMedia()
	})
	res := e.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.CredentialAccount
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	avatar, mediaID, followers, following := processedVariants(avatarID), avatarID, 2, 1
	want := clientapi.CredentialAccount{
		Id: aliceAccountID, Username: "alice", Acct: "alice", DisplayName: "Alice", Note: "<p>hi</p>",
		Url: "https://social.example/@alice", Locked: true, Discoverable: true, CreatedAt: createdAt, Avatar: &avatar,
		FollowersCount: &followers, FollowingCount: &following, Emojis: []clientapi.CustomEmoji{}, Badges: []clientapi.AccountBadge{},
		Fields: []clientapi.ProfileField{{Name: "web", Value: `<p><a href="https://alice.example">alice.example</a></p>`}},
		Source: clientapi.AccountSource{
			Note: "hi", Fields: []clientapi.AccountFieldSource{{Name: "web", Value: "https://alice.example"}}, AvatarMediaId: &mediaID,
		},
		Posting: clientapi.PostingLimits{
			VisibilityCeiling: clientapi.VisibilityPublic,
			Visibilities: []clientapi.Visibility{
				clientapi.VisibilityPublic, clientapi.VisibilityLocal, clientapi.VisibilityUnlisted, clientapi.VisibilityPrivate, clientapi.VisibilityDirect,
			},
		},
		Permissions: []clientapi.Permission{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("account = %+v, want %+v", got, want)
	}
}

// fakePosting answers the posting limits of a silenced account (data-model §5.4), or err.
type fakePosting struct{ err error }

func (f fakePosting) PostingLimits(_ context.Context, accountID string) (appcontent.PostingLimits, error) {
	if f.err != nil {
		return appcontent.PostingLimits{}, f.err
	}
	if accountID != aliceAccountID {
		return appcontent.PostingLimits{}, errors.New("unexpected account " + accountID)
	}
	silenced := content.AuthorRestrictions{VisibilityCeilingPrivate: true}
	return appcontent.PostingLimits{VisibilityCeiling: silenced.VisibilityCeiling(), Visibilities: content.PostingVisibilities(silenced)}, nil
}

func TestCredentialAccountHasThePostingLimits(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Posting = fakePosting{} })
	res := e.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.CredentialAccount
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := clientapi.PostingLimits{
		VisibilityCeiling: clientapi.VisibilityPrivate,
		Visibilities:      []clientapi.Visibility{clientapi.VisibilityPrivate, clientapi.VisibilityDirect},
	}
	if !reflect.DeepEqual(got.Posting, want) {
		t.Errorf("posting = %+v, want %+v", got.Posting, want)
	}

	failing := newAuthEnv(t, func(o *clientapi.Options) { o.Posting = fakePosting{err: errors.New("store down")} })
	if res := failing.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer read"}); res.StatusCode != http.StatusInternalServerError {
		t.Errorf("status with a failing policy = %d, want 500", res.StatusCode)
	}
}

// fakePostCounts answers the post count of alice, or err.
type fakePostCounts struct {
	n   int64
	err error
}

func (f fakePostCounts) PostCount(_ context.Context, accountID string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	if accountID != aliceAccountID {
		return 0, errors.New("unexpected account " + accountID)
	}
	return f.n, nil
}

func TestCredentialAccountHasThePostCount(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) { o.PostCounts = fakePostCounts{n: 3} })
	res := e.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.CredentialAccount
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.StatusesCount != 3 {
		t.Errorf("statuses_count = %d, want 3", got.StatusesCount)
	}

	failing := newAuthEnv(t, func(o *clientapi.Options) { o.PostCounts = fakePostCounts{err: errors.New("store down")} })
	if res := failing.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer read"}); res.StatusCode != http.StatusInternalServerError {
		t.Errorf("status with a failing count = %d, want 500", res.StatusCode)
	}
}

func TestUpdateAccountCredentials(t *testing.T) {
	profiles := &fakeProfiles{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		alice := o.Accounts.(fakeAccounts)[aliceAccountID]
		alice.Profile = aliceProfile()
		profiles.actor = alice
		o.Profiles = profiles
		o.Media = profileMedia()
	})
	res := e.sendJSON(t, http.MethodPatch, "/accounts/update_credentials", "write",
		`{"display_name":"Alice A.","note":"hello","fields":[{"name":"pronouns","value":"she/her"}],"bot":true}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var got clientapi.CredentialAccount
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Alice A." || got.Note != "<p>hello</p>" || got.Source.Note != "hello" ||
		len(got.Fields) != 1 || got.Fields[0].Name != "pronouns" || got.Source.Fields[0].Value != "she/her" {
		t.Errorf("account = %+v, want the new profile", got)
	}
	if len(profiles.got) != 1 {
		t.Fatalf("UpdateProfile called %d times", len(profiles.got))
	}
	in := profiles.got[0]
	if in.Bot == nil || !*in.Bot || in.Locked != nil || in.AvatarMediaID != nil || in.Fields == nil {
		t.Errorf("update = %+v, want only the sent properties", in)
	}

	// write:accounts is needed; read and follow do not grant it.
	for _, token := range []string{"read", "follow-push"} {
		if res := e.sendJSON(t, http.MethodPatch, "/accounts/update_credentials", token, `{}`); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", token, res.StatusCode)
		}
	}
	if res := e.sendJSON(t, http.MethodPatch, "/accounts/update_credentials", "", `{}`); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous: status = %d, want 401", res.StatusCode)
	}
}

func TestUpdateAccountCredentialsProblems(t *testing.T) {
	profiles := &fakeProfiles{}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Profiles = profiles })
	for _, tt := range []struct {
		err        error
		wantDetail string
	}{
		// alice's locale is ja.
		{&account.ProfileError{Property: account.PropertyDisplayName, Problem: account.ProblemTooLong, Max: 30},
			"display_name は 30 文字以内にしてください。"},
		{&account.ProfileError{Property: account.PropertyFields, Problem: account.ProblemTooMany, Max: 4},
			"fields は 4 個までにしてください。"},
		{&account.ProfileError{Property: account.PropertyFieldName, Problem: account.ProblemInvalid},
			"fields[].name の値が正しくありません。"},
		{&account.ProfileError{Property: account.PropertyAvatar, Problem: account.ProblemInvalid},
			"avatar_media_id には、自分でアップロードして処理が終わった画像を指定してください。"},
	} {
		profiles.err = tt.err
		res := e.sendJSON(t, http.MethodPatch, "/accounts/update_credentials", "write", `{"display_name":"x"}`)
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%v: status = %d, want 422", tt.err, res.StatusCode)
		}
		if lang := res.Header.Get("Content-Language"); lang != "ja" {
			t.Errorf("%v: Content-Language = %q, want ja", tt.err, lang)
		}
		if p := decodeProblem(t, res); p.Detail == nil || *p.Detail != tt.wantDetail || p.Type != clientapi.ProblemTypeBlank {
			t.Errorf("%v: problem = %+v (detail %v), want %q", tt.err, p, p.Detail, tt.wantDetail)
		}
	}
	// A body that is not JSON is a 400.
	if res := e.sendJSON(t, http.MethodPatch, "/accounts/update_credentials", "write", `invalid-on-purpose`); res.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", res.StatusCode)
	}
}
