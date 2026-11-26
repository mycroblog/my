// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/mycroblog/my/internal/adapter/voice/sfu/node"
	"github.com/mycroblog/my/internal/adapter/voice/sfu/node/nodetest"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

// sfuProcess is an App of voice = sfu served over HTTP on loopback, with its media node, if its
// role runs one, started on loopback ports, and the measurements of its metrics.
type sfuProcess struct {
	app   *platform.App
	srv   *httptest.Server
	meter *testkit.Meter
	// url is the node's WebSocket (voice_sfu_public_url), which its grants name.
	url string
}

// newSFUProcess builds the App of role on env with voice = sfu, serves its handler, and starts its
// media node when it has one. env has the database, the stream, and the web origin.
func newSFUProcess(t *testing.T, env map[string]string, role string) *sfuProcess {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "ws://" + l.Addr().String() + config.VoiceSFUPath
	full := map[string]string{
		"MY_ROLE": role, "MY_VOICE": "sfu",
		"MY_VOICE_SFU_UDP_PORT": strconv.Itoa(freePort(t, "udp")), "MY_VOICE_SFU_TCP_PORT": strconv.Itoa(freePort(t, "tcp")),
		"MY_VOICE_SFU_PUBLIC_IP": "127.0.0.1", "MY_VOICE_SFU_PUBLIC_URL": url,
	}
	for k, v := range env {
		full[k] = v
	}
	meter := testkit.NewMeter()
	app, err := platform.NewApp(t.Context(), loadConfig(t, full), platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Meter: meter})
	if err != nil {
		_ = l.Close()
		t.Fatalf("NewApp(%s): %v", role, err)
	}
	t.Cleanup(func() { _ = app.Close() })
	srv := httptest.NewUnstartedServer(app.Handler())
	_ = srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	if app.VoiceNode != nil {
		if err := app.VoiceNode.Start(t.Context()); err != nil {
			t.Fatalf("Start the media node of %s: %v", role, err)
		}
		t.Cleanup(func() { _ = app.VoiceNode.Close(context.Background()) })
	}
	return &sfuProcess{app: app, srv: srv, meter: meter, url: url}
}

// packets returns the RTP packets that the process's node counted with outcome.
func (p *sfuProcess) packets(outcome string) float64 {
	return p.observed(node.Packets.Name, map[string]string{"my.outcome": outcome})
}

// observed collects the gauges and the observable counters of the process's meter and returns
// the last value of the metric name with the labels of match, -1 when it has none.
func (p *sfuProcess) observed(name string, match map[string]string) float64 {
	if err := p.meter.Collect(context.Background()); err != nil {
		return -1
	}
	ms := p.meter.Measurements(name, match)
	if len(ms) == 0 {
		return -1
	}
	return ms[len(ms)-1].Value
}

// connectSFU connects session to the node of the grant g.
func connectSFU(t *testing.T, g clientapi.VoiceGrant, session string) *nodetest.Client {
	t.Helper()
	if g.Kind != clientapi.VoiceGrantKindSfu || g.Url == nil || g.Token == nil {
		t.Fatalf("grant = %+v, want an sfu grant with a URL and a ticket", g)
	}
	return nodetest.Connect(t, *g.Url, *g.Token, session)
}

// refreshGrant asks the App of e for a new grant of session (POST /voice/rooms/{id}/grant).
func refreshGrant(e *accountsEnv, token, room, session string) clientapi.VoiceGrant {
	e.t.Helper()
	var g clientapi.VoiceGrant
	e.want(e.do(http.MethodPost, "/voice/rooms/"+room+"/grant", token, map[string]any{"session": session}, &g), http.StatusOK, "grant")
	return g
}
