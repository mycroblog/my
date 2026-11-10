// SPDX-License-Identifier: AGPL-3.0-only

package timeline_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// filterRepository answers ActiveKeywordFilters with the filters the test gives it, and records
// the context and accounts it was asked for.
type filterRepository struct {
	apptimeline.Repository
	filters  map[string][]timeline.KeywordFilter
	asked    []string
	contexts []timeline.FilterContext
}

func (r *filterRepository) ActiveKeywordFilters(_ context.Context, ids []string, c timeline.FilterContext, _ time.Time) (map[string][]timeline.KeywordFilter, error) {
	r.asked, r.contexts = append(r.asked, ids...), append(r.contexts, c)
	out := map[string][]timeline.KeywordFilter{}
	for _, id := range ids {
		if fs, ok := r.filters[id]; ok {
			out[id] = fs
		}
	}
	return out, nil
}

func filterWords(t *testing.T, words ...string) []timeline.FilterWord {
	t.Helper()
	var out []timeline.FilterWord
	for _, w := range words {
		fw, err := timeline.NewFilterWord(w, false, apptimeline.NFKC, apptimeline.Graphemes)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, fw)
	}
	return out
}

func statusView(id, author string, html ...string) appcontent.StatusView {
	v := appcontent.StatusView{Status: content.Status{ID: id, AuthorAccountID: author}}
	for _, h := range html {
		v.Revision.Bodies = append(v.Revision.Bodies, content.Body{Language: "en", ContentHTML: h})
	}
	return v
}

func TestStatusFilterJudge(t *testing.T) {
	warn := timeline.KeywordFilter{ID: testID(1), AccountID: viewerID, Title: "Spoilers", Action: timeline.FilterWarn, Words: filterWords(t, "Spoiler", "ネタバレ")}
	hide := timeline.KeywordFilter{ID: testID(2), AccountID: viewerID, Title: "Ads", Action: timeline.FilterHide, Words: filterWords(t, "SALE")}
	repo := &filterRepository{filters: map[string][]timeline.KeywordFilter{viewerID: {warn, hide}}}
	svc := apptimeline.NewService(apptimeline.Deps{Repository: repo, Clock: fixedClock{time.Now()}})
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: viewerID}
	f, err := svc.StatusFilter(t.Context(), viewer, timeline.FilterContextHome)
	if err != nil || f == nil {
		t.Fatalf("StatusFilter = %v, %v", f, err)
	}
	if repo.contexts[0] != timeline.FilterContextHome {
		t.Errorf("asked for %v, want home", repo.contexts)
	}

	withCW := statusView(testID(10), authorID, "<p>nothing here</p>")
	withCW.Revision.SpoilerText = "ｓｐｏｉｌｅｒ ahead" // full-width Latin, matched through NFKC
	poll := statusView(testID(11), authorID, "<p>vote</p>")
	poll.Poll = &appcontent.PollView{Poll: content.Poll{Options: []content.PollOption{{Title: "yes"}, {Title: "ﾈﾀﾊﾞﾚ"}}}} // half-width katakana
	alt := statusView(testID(12), authorID, "<p>a picture</p>")
	alt.Attachments = []appcontent.MediaView{{Description: "a big sale banner"}}
	link := statusView(testID(13), authorID, `<p><a href="https://sale.example/">a link</a></p>`)
	second := statusView(testID(14), authorID, "<p>first language</p>", "<p>une ネタバレ</p>")
	quoting := statusView(testID(15), authorID, "<p>look at this</p>")
	quoted := statusView(testID(16), testID(17), "<p>huge sale</p>")
	quoting.Quote = &appcontent.QuoteView{State: appcontent.QuoteViewAccepted, Status: &quoted}
	own := statusView(testID(18), viewerID, "<p>my own spoiler</p>")
	ownQuote := statusView(testID(19), viewerID, "<p>me</p>")
	ownQuote.Quote = &appcontent.QuoteView{State: appcontent.QuoteViewAccepted, Status: &quoted}
	boosted := statusView(testID(20), authorID, "<p>spoiler</p>")

	warned := []timeline.FilterMatch{{Filter: warn, Words: []string{"Spoiler"}}}
	cases := []struct {
		name    string
		v       appcontent.StatusView
		booster string
		want    apptimeline.Judgement
	}{
		{"content warning", withCW, "", apptimeline.Judgement{Matches: warned}},
		{"poll choice", poll, "", apptimeline.Judgement{Matches: []timeline.FilterMatch{{Filter: warn, Words: []string{"ネタバレ"}}}}},
		{"alt text hides", alt, "", apptimeline.Judgement{Matches: []timeline.FilterMatch{{Filter: hide, Words: []string{"SALE"}}}, Hide: true}},
		{"attributes are not text", link, "", apptimeline.Judgement{}},
		{"every language", second, "", apptimeline.Judgement{Matches: []timeline.FilterMatch{{Filter: warn, Words: []string{"ネタバレ"}}}}},
		{"a quote folds only the quote", quoting, "", apptimeline.Judgement{Quote: []timeline.FilterMatch{{Filter: hide, Words: []string{"SALE"}}}}},
		{"own post", own, "", apptimeline.Judgement{}},
		{"what the viewer quotes", ownQuote, "", apptimeline.Judgement{}},
		{"own boost", boosted, viewerID, apptimeline.Judgement{}},
		{"a boost by another", boosted, testID(30), apptimeline.Judgement{Matches: warned}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.Judge(tc.v, tc.booster); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Judge = %+v, want %+v", got, tc.want)
			}
		})
	}

	var none *apptimeline.StatusFilter
	if got := none.Judge(withCW, ""); !reflect.DeepEqual(got, apptimeline.Judgement{}) {
		t.Errorf("nil Judge = %+v", got)
	}
}

func TestStatusFilterOfVisitorsAndViewersWithout(t *testing.T) {
	repo := &filterRepository{filters: map[string][]timeline.KeywordFilter{
		viewerID: {{ID: testID(1), AccountID: viewerID, Title: "x", Action: timeline.FilterHide, Words: filterWords(t, "x")}},
	}}
	svc := apptimeline.NewService(apptimeline.Deps{Repository: repo, Clock: fixedClock{time.Now()}})
	if f, err := svc.StatusFilter(t.Context(), appcontent.Viewer{}, timeline.FilterContextPublic); err != nil || f != nil {
		t.Errorf("StatusFilter(visitor) = %v, %v; want nil", f, err)
	}
	if len(repo.asked) != 0 {
		t.Errorf("a visitor read the filters of %v", repo.asked)
	}
	other := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: authorID}
	if f, err := svc.StatusFilter(t.Context(), other, timeline.FilterContextPublic); err != nil || f != nil {
		t.Errorf("StatusFilter(without filters) = %v, %v; want nil", f, err)
	}
	fs, err := svc.StatusFiltersOf(t.Context(), []string{viewerID, authorID, viewerID}, timeline.FilterContextPublic)
	if err != nil || len(fs) != 1 || fs[viewerID] == nil {
		t.Errorf("StatusFiltersOf = %v, %v; want the viewer's", fs, err)
	}
	if want := []string{viewerID, authorID}; !slices.Equal(repo.asked[len(repo.asked)-2:], want) {
		t.Errorf("asked for %v, want each account once", repo.asked)
	}
}

// statusViews answers Views with the statuses the test gives it, as visible.
type statusViews map[string]appcontent.StatusView

func (s statusViews) Views(_ context.Context, _ appcontent.Viewer, ids []string) ([]appcontent.StatusView, error) {
	var out []appcontent.StatusView
	for _, id := range ids {
		if v, ok := s[id]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func TestJudgeStatuses(t *testing.T) {
	hide := timeline.KeywordFilter{ID: testID(2), AccountID: viewerID, Title: "Ads", Action: timeline.FilterHide, Words: filterWords(t, "sale")}
	repo := &filterRepository{filters: map[string][]timeline.KeywordFilter{viewerID: {hide}}}
	views := statusViews{
		testID(10): statusView(testID(10), authorID, "<p>a sale</p>"),
		testID(11): statusView(testID(11), authorID, "<p>nothing</p>"),
	}
	svc := apptimeline.NewService(apptimeline.Deps{Repository: repo, Clock: fixedClock{time.Now()}, Statuses: views})
	got, err := svc.JudgeStatuses(t.Context(), viewerID, timeline.FilterContextNotifications, []string{testID(10), testID(11), testID(12)})
	if err != nil || len(got) != 1 || !got[testID(10)].Hide {
		t.Errorf("JudgeStatuses = %+v, %v; want the first hidden", got, err)
	}
	if repo.contexts[0] != timeline.FilterContextNotifications {
		t.Errorf("asked for %v", repo.contexts)
	}
	// A viewer without filters reads no status.
	svc = apptimeline.NewService(apptimeline.Deps{Repository: repo, Clock: fixedClock{time.Now()}})
	if got, err := svc.JudgeStatuses(t.Context(), authorID, timeline.FilterContextNotifications, []string{testID(10)}); err != nil || len(got) != 0 {
		t.Errorf("JudgeStatuses(without filters) = %+v, %v", got, err)
	}
}
