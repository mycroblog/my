// SPDX-License-Identifier: AGPL-3.0-only

package connect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	hookconnect "github.com/mycroblog/my/internal/adapter/hook/connect"
	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	hookv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1/hookv1connect"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/pkg/testkit"
)

// behaviour is what the plugins of these tests do next.
type behaviour struct {
	answer apphook.Answer
	broken bool
	fail   bool
	block  bool
}

var (
	verdicts = map[hook.Outcome]hookv1.Verdict{
		hook.Allow: hookv1.Verdict_VERDICT_ALLOW, hook.Drop: hookv1.Verdict_VERDICT_DROP, hook.Flag: hookv1.Verdict_VERDICT_FLAG,
		hook.MarkSensitive: hookv1.Verdict_VERDICT_MARK_SENSITIVE, hook.RequireApproval: hookv1.Verdict_VERDICT_REQUIRE_APPROVAL,
		hook.Reject: hookv1.Verdict_VERDICT_REJECT,
	}
	reasons = map[hook.Reason]hookv1.Reason{hook.ReasonSpam: hookv1.Reason_REASON_SPAM}
)

type connectPlugin struct {
	hookv1connect.UnimplementedHookServiceHandler

	next *atomic.Pointer[behaviour]
}

func (p *connectPlugin) Check(ctx context.Context, _ *hookv1.CheckRequest) (*hookv1.CheckResponse, error) {
	b := p.next.Load()
	switch {
	case b.block:
		<-ctx.Done()
		return nil, ctx.Err()
	case b.fail:
		return nil, connect.NewError(connect.CodeUnavailable, "the server details")
	case b.broken:
		return &hookv1.CheckResponse{}, nil
	}
	return &hookv1.CheckResponse{Verdict: verdicts[b.answer.Verdict], Reason: reasons[b.answer.Reason], Note: b.answer.Note}, nil
}

// jsonPlugin is a plugin of net/http and encoding/json alone.
func jsonPlugin(next *atomic.Pointer[behaviour]) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /my.plugin.hook.v1.HookService/Check", func(w http.ResponseWriter, r *http.Request) {
		b := next.Load()
		switch {
		case b.block:
			<-r.Context().Done()
			return
		case b.fail:
			http.Error(w, "the server's details", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if b.broken {
			_, _ = w.Write([]byte("{}"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"verdict": verdicts[b.answer.Verdict].String(), "reason": reasons[b.answer.Reason].String(), "note": b.answer.Note,
		})
	})
	return mux
}

func TestContract(t *testing.T) {
	newConnect := func(next *atomic.Pointer[behaviour]) http.Handler {
		server := connect.NewServer()
		hookv1connect.RegisterHookServiceHandler(server, &connectPlugin{next: next})
		mux := http.NewServeMux()
		connecthttp.Mount(mux, server)
		return mux
	}
	for name, c := range map[string]struct {
		handler  func(*atomic.Pointer[behaviour]) http.Handler
		protocol string
	}{
		"connect json":  {newConnect, kind.ProtocolJSON},
		"connect proto": {newConnect, kind.ProtocolProto},
		"connect grpc":  {newConnect, kind.ProtocolGRPC},
		"plain json":    {jsonPlugin, kind.ProtocolJSON},
	} {
		t.Run(name, func(t *testing.T) {
			testkit.HookPluginContract(t, func(t *testing.T) testkit.HookPluginHarness {
				next := new(atomic.Pointer[behaviour])
				next.Store(&behaviour{answer: apphook.Answer{Verdict: hook.Allow}})
				k, ok := kind.ByName(kind.InboundStatus)
				if !ok {
					t.Fatal("no kind")
				}
				conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{
					Address: fakeplugin.ServeUnix(t, c.handler(next)), Protocol: c.protocol, Timeout: 500 * time.Millisecond,
				}})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(conn.Close)
				return testkit.HookPluginHarness{
					Plugin: hookconnect.New(conn),
					Answer: func(a apphook.Answer) { next.Store(&behaviour{answer: a}) },
					Broken: func() { next.Store(&behaviour{broken: true}) },
					Fail:   func() { next.Store(&behaviour{fail: true}) },
					Block:  func() { next.Store(&behaviour{block: true}) },
				}
			})
		})
	}
}

// The subject reaches the plugin in the protocol's own fields, and the text goes over no plain TCP.
func TestInboundStatusSubject(t *testing.T) {
	t.Parallel()
	got := make(chan *hookv1.CheckRequest, 1)
	server := connect.NewServer()
	hookv1connect.RegisterHookServiceHandler(server, &subjectPlugin{got: got})
	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)
	conn, err := client.New(client.Options{
		Kind: mustKind(t, kind.InboundStatus), Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, mux), Protocol: kind.ProtocolProto},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	published := time.Date(2026, 11, 6, 1, 2, 3, 0, time.UTC)
	_, err = hookconnect.New(conn).Check(t.Context(), apphook.Request{
		Hook: hook.InboundStatus, SubjectKey: "https://remote.example/a",
		Subject: apphook.InboundStatusSubject{
			ActivityType: "Update", StatusURI: "https://remote.example/s", AuthorURI: "https://remote.example/u", AuthorDomain: "remote.example",
			Visibility: "direct", AnyLocalFollower: true, PublishedAt: published, MentionCount: 3, LocalMentionCount: 2,
			Bodies:      []apphook.InboundStatusBody{{Language: "en", HTML: "<p>hello</p>"}},
			Attachments: []apphook.InboundStatusAttachment{{Type: "Image", URL: "https://remote.example/i.png", ContentType: "image/png", Description: "a cat"}},
			PollOptions: []string{"yes"}, Hashtags: []string{"tag"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := (<-got).GetInboundStatus()
	if s.GetActivityType() != "Update" || s.GetStatusUri() != "https://remote.example/s" || s.GetVisibility() != "direct" || !s.GetAnyLocalFollower() ||
		s.GetPublishedAt() != "2026-11-06T01:02:03Z" || s.GetMentionCount() != 3 || s.GetLocalMentionCount() != 2 ||
		s.GetBodies()[0].GetHtml() != "<p>hello</p>" || s.GetAttachments()[0].GetDescription() != "a cat" || s.GetPollOptions()[0] != "yes" || s.GetHashtags()[0] != "tag" {
		t.Errorf("subject = %v", s)
	}
}

type subjectPlugin struct {
	hookv1connect.UnimplementedHookServiceHandler

	got chan *hookv1.CheckRequest
}

func (p *subjectPlugin) Check(_ context.Context, r *hookv1.CheckRequest) (*hookv1.CheckResponse, error) {
	p.got <- r
	return &hookv1.CheckResponse{Verdict: hookv1.Verdict_VERDICT_ALLOW}, nil
}

func mustKind(t *testing.T, name string) kind.Kind {
	t.Helper()
	k, ok := kind.ByName(name)
	if !ok {
		t.Fatalf("no kind %s", name)
	}
	return k
}

// media.check sends the processed image as bytes, which a plugin of JSON alone reads as base64, and
// may use plain TCP: it sends no text of a post.
func TestMediaSubject(t *testing.T) {
	t.Parallel()
	got := make(chan map[string]any, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /my.plugin.hook.v1.HookService/Check", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		got <- req
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"verdict":"VERDICT_REJECT"}`))
	})
	conn, err := client.New(client.Options{
		Kind: mustKind(t, kind.MediaCheck), Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, mux)},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	a, err := hookconnect.New(conn).Check(t.Context(), apphook.Request{
		Hook: hook.MediaCheck, SubjectKey: "media-1",
		Subject: apphook.MediaSubject{
			Purpose: "attachment", MediaType: "video", Image: []byte("webp"), ImageContentType: "image/webp", ImageWidth: 64, ImageHeight: 36,
			Width: 1280, Height: 720, Duration: 1500 * time.Millisecond, Animated: true,
		},
	})
	if err != nil || a.Verdict != hook.Reject {
		t.Fatalf("Check = %+v, %v", a, err)
	}
	m, _ := (<-got)["media"].(map[string]any)
	want := map[string]any{
		"purpose": "attachment", "mediaType": "video", "image": "d2VicA==", "imageContentType": "image/webp", "imageWidth": 64.0,
		"imageHeight": 36.0, "width": 1280.0, "height": 720.0, "durationMs": "1500", "animated": true,
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("media.%s = %v, want %v", k, m[k], v)
		}
	}
	if _, err := client.New(client.Options{Kind: mustKind(t, kind.MediaCheck), Destination: kind.Destination{Address: "http://plugin.example"}}); err != nil {
		t.Errorf("media.check refuses plain TCP: %v", err)
	}
}

// inbound.status sends the text of posts, so plain TCP is refused, on loopback too.
func TestInboundStatusRefusesPlainTCP(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"http://127.0.0.1:1", "http://localhost:1", "http://plugin.example"} {
		if _, err := client.New(client.Options{Kind: mustKind(t, kind.InboundStatus), Destination: kind.Destination{Address: addr}}); err == nil {
			t.Errorf("client.New(%s) took plain TCP", addr)
		}
	}
}
