// SPDX-License-Identifier: AGPL-3.0-only

// Package elasticsearchtest gives tests an Elasticsearch node for the contract of the
// search = elasticsearch index (ADR-0149 §14): the adapter checked against the real engine rather
// than the httptest stand-ins of its own tests. The node has security on, with the password of
// the user elastic, so that the tests send credentials as a real cluster needs, and plain HTTP.
//
// Each New starts a container of its own. The tests need Docker and run only with
// TEST_SEARCH=container (scripts/task.sh go:test-search; CI in the slow suite): without it New
// skips the test, and with it a Docker that cannot start the container fails the test.
// TEST_ELASTICSEARCH_IMAGE runs them on another image, such as one of Elasticsearch 8.
package elasticsearchtest

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
	EnvSearch = "TEST_SEARCH"
	EnvImage  = "TEST_ELASTICSEARCH_IMAGE"
)

// DefaultImage is the container image of the tests, pinned by tag and digest.
const DefaultImage = "elasticsearch:9.5.3@sha256:e23d4758358a4e356cc2ef3259a5a6f345cacc1722c10dffe6d7150a1a78e52f"

// The user and password of the node.
const (
	Username = "elastic"
	Password = "a-password-for-tests"
)

// New starts a node for t and stops it when t ends, returning the URL of its REST API. It skips t
// unless TEST_SEARCH is "container".
func New(t testing.TB) string {
	t.Helper()
	if os.Getenv(EnvSearch) != "container" {
		t.Skipf("the Elasticsearch tests run with %s=container (scripts/task.sh go:test-search)", EnvSearch)
	}
	image := os.Getenv(EnvImage)
	if image == "" {
		image = DefaultImage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("9200/tcp"),
		testcontainers.WithEnv(map[string]string{
			"discovery.type":                       "single-node",
			"xpack.security.enabled":               "true",
			"xpack.security.http.ssl.enabled":      "false",
			"xpack.security.transport.ssl.enabled": "false",
			"ELASTIC_PASSWORD":                     Password,
			"ES_JAVA_OPTS":                         "-Xms512m -Xmx512m",
			// A test machine's disk is often fuller than the watermarks allow.
			"cluster.routing.allocation.disk.threshold_enabled": "false",
		}),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/_cluster/health?wait_for_status=yellow").
			WithPort("9200/tcp").WithBasicAuth(Username, Password).WithStartupTimeout(3*time.Minute)),
	)
	if c != nil {
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	}
	if err != nil {
		t.Fatalf("elasticsearchtest: start container %s: %v", image, err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("elasticsearchtest: container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "9200/tcp")
	if err != nil {
		t.Fatalf("elasticsearchtest: port: %v", err)
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(int(port.Num())))
}
