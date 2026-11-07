// SPDX-License-Identifier: AGPL-3.0-only

// Package meilisearchtest gives tests a Meilisearch server (MIT) for the contract of the
// search = meilisearch index: the adapter's calls checked against the real engine rather than the
// httptest stand-in of its own tests.
//
// One server serves every test of a test binary, and each test makes its own index on it.
// Environment variables choose the server:
//
//   - TEST_MEILISEARCH_URL (with TEST_MEILISEARCH_KEY, its master key): an existing server; the
//     tests make indexes with random uids there and delete them at the end.
//   - Without it, the tests run only with TEST_SEARCH=container (scripts/task.sh go:test-search;
//     CI in the slow suite), on a container of TEST_MEILISEARCH_IMAGE (default DefaultImage)
//     started with testcontainers-go. Without either the tests skip, and with TEST_SEARCH=container
//     a Docker that cannot start the container fails them.
//
// The TestMain of a package that uses meilisearchtest calls os.Exit(meilisearchtest.Main(m)) to
// stop the container.
package meilisearchtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The environment variables that choose the server; see the package documentation.
const (
	EnvSearch = "TEST_SEARCH"
	EnvURL    = "TEST_MEILISEARCH_URL"
	EnvKey    = "TEST_MEILISEARCH_KEY"
	EnvImage  = "TEST_MEILISEARCH_IMAGE"
)

// DefaultImage is the container image of the tests, pinned by tag and digest.
const DefaultImage = "getmeili/meilisearch:v1.56.1@sha256:b7ba431bf6f9b26cc86132798f3640964147a699cf23f026b74c6c05dbb1d60d"

// masterKey is the master key of the container: Meilisearch requires one of 16 bytes or more.
const masterKey = "meilisearchtest-master-key"

var server struct {
	once sync.Once
	url  string
	key  string
	skip string
	err  error
	stop func()
}

// Main runs the tests of m and then stops the container, if a test started one.
func Main(m *testing.M) int {
	code := m.Run()
	if server.stop != nil {
		server.stop()
	}
	return code
}

// Server is the server of one test.
type Server struct {
	// URL is the base URL of the server, and Key its master key.
	URL string
	Key string
	// Index is the test's own index uid, which does not exist yet and is deleted when the test
	// ends.
	Index string
}

// New returns the test server with a new index uid. It skips t when no server is configured.
func New(t testing.TB) Server {
	t.Helper()
	server.once.Do(setUp)
	if server.err != nil {
		t.Fatalf("meilisearchtest: %v", server.err)
	}
	if server.url == "" {
		t.Skip(server.skip)
	}
	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	s := Server{URL: server.url, Key: server.key, Index: "test_" + hex.EncodeToString(suffix)}
	t.Cleanup(func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, s.URL+"/indexes/"+s.Index, nil)
		if err != nil {
			return
		}
		if s.Key != "" {
			req.Header.Set("Authorization", "Bearer "+s.Key)
		}
		if res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req); err == nil {
			_ = res.Body.Close()
		}
	})
	return s
}

func setUp() {
	if u := os.Getenv(EnvURL); u != "" {
		server.url, server.key = u, os.Getenv(EnvKey)
		return
	}
	if os.Getenv(EnvSearch) != "container" {
		server.skip = fmt.Sprintf("the Meilisearch tests run with %s=container (scripts/task.sh go:test-search) or %s", EnvSearch, EnvURL)
		return
	}
	image := os.Getenv(EnvImage)
	if image == "" {
		image = DefaultImage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("7700/tcp"),
		testcontainers.WithEnv(map[string]string{"MEILI_MASTER_KEY": masterKey, "MEILI_NO_ANALYTICS": "true", "MEILI_ENV": "production"}),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(2*time.Minute)),
	)
	if c != nil {
		server.stop = func() { _ = testcontainers.TerminateContainer(c) }
	}
	if err != nil {
		server.err = fmt.Errorf("start container %s: %w", image, err)
		return
	}
	endpoint, err := c.PortEndpoint(ctx, "7700/tcp", "http")
	if err != nil {
		server.err = fmt.Errorf("container endpoint: %w", err)
		return
	}
	server.url, server.key = endpoint, masterKey
}
