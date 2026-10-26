// SPDX-License-Identifier: AGPL-3.0-only

// Package livekittest gives tests a LiveKit server (livekit-server --dev, Apache-2.0) for the
// contract of the voice = livekit transport (ADR-0102 §2): the adapter's calls of the Twirp API
// checked against the real server rather than the httptest stand-in of its own tests.
//
// Each New starts a container of its own. The tests need Docker and run only with
// TEST_LIVEKIT=container (scripts/task.sh go:test-voice; CI in the slow suite): without it New
// skips the test, and with it a Docker that cannot start the container fails the test.
package livekittest

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The environment variables of the tests; see the package documentation.
const (
	EnvLiveKit = "TEST_LIVEKIT"
	EnvImage   = "TEST_LIVEKIT_IMAGE"
)

// DefaultImage is the container image of the tests, pinned by tag and digest.
const DefaultImage = "livekit/livekit-server:v1.13.10@sha256:89ca9ce0791e18d6e03bf6e2c49ceb27e1c754eae8bd50c989386c1a5963791a"

// The API key and secret of livekit-server --dev.
const (
	APIKey    = "devkey"
	APISecret = "secret"
)

// Server is the LiveKit server of one test.
type Server struct {
	// URL is the ws: URL of the server, and APIURL the http: URL of its API.
	URL    string
	APIURL string
}

// New starts a server for t and stops it when t ends. It skips t unless TEST_LIVEKIT is
// "container".
func New(t testing.TB) Server {
	t.Helper()
	if os.Getenv(EnvLiveKit) != "container" {
		t.Skipf("the LiveKit server tests run with %s=container (scripts/task.sh go:test-voice)", EnvLiveKit)
	}
	image := os.Getenv(EnvImage)
	if image == "" {
		image = DefaultImage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("7880/tcp"),
		testcontainers.WithCmdArgs("--dev", "--bind", "0.0.0.0"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/").WithPort("7880/tcp").WithStartupTimeout(time.Minute)),
	)
	if c != nil {
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	}
	if err != nil {
		t.Fatalf("livekittest: start container %s: %v", image, err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("livekittest: container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "7880/tcp")
	if err != nil {
		t.Fatalf("livekittest: port: %v", err)
	}
	addr := net.JoinHostPort(host, strconv.Itoa(int(port.Num())))
	return Server{URL: "ws://" + addr, APIURL: "http://" + addr}
}
