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

// recordingPlugin answers allow and keeps the last request.
type recordingPlugin struct {
	hookv1connect.UnimplementedHookServiceHandler

	last atomic.Pointer[hookv1.CheckRequest]
}

func (p *recordingPlugin) Check(_ context.Context, req *hookv1.CheckRequest) (*hookv1.CheckResponse, error) {
	p.last.Store(req)
	return &hookv1.CheckResponse{Verdict: hookv1.Verdict_VERDICT_ALLOW}, nil
}

func TestTheSubjectOfLocalStatus(t *testing.T) {
	plugin := new(recordingPlugin)
	server := connect.NewServer()
	hookv1connect.RegisterHookServiceHandler(server, plugin)
	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)
	k, ok := kind.ByName(kind.LocalStatus)
	if !ok {
		t.Fatal("no kind")
	}
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, mux), Protocol: kind.ProtocolJSON}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	created := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)
	_, err = hookconnect.New(conn).Check(t.Context(), apphook.Request{
		Hook: hook.LocalStatus, SubjectKey: "status-id",
		Subject: apphook.LocalStatusSubject{
			StatusURI: "https://my.example/ap/objects/status-id", AuthorURI: "https://my.example/ap/actors/a", AuthorUsername: "alice",
			AccountCreatedAt: created, Visibility: "public", Language: "en", Text: "buy now", HTML: "<p>buy now</p>",
			AttachmentDescriptions: []string{"", "a cat"}, Hashtags: []string{"deal"}, MentionCount: 3,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sent := plugin.last.Load()
	got := sent.GetLocalStatus()
	if got == nil || got.GetText() != "buy now" || got.GetHtml() != "<p>buy now</p>" || got.GetAuthorUsername() != "alice" ||
		got.GetAccountCreatedAt() != "2026-11-01T09:30:00Z" || got.GetMentionCount() != 3 || len(got.GetAttachmentDescriptions()) != 2 ||
		len(got.GetHashtags()) != 1 || got.GetVisibility() != "public" {
		t.Errorf("sent %v", got)
	}
	if sent.GetSignup() != nil || sent.GetFollow() != nil {
		t.Error("another hook's subject was sent")
	}
}
