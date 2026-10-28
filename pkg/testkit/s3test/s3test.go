// SPDX-License-Identifier: AGPL-3.0-only

// Package s3test gives tests an S3-compatible server for the contract of the blob = s3 store:
// the adapter's calls checked against a real server rather than the in-memory stand-in of its
// own tests.
//
// The server is Garage (AGPL-3.0), an S3-compatible object store whose keys are a flat
// namespace like Amazon S3, R2, and Cloud Storage: MinIO and the S3 gateways on a file system
// store a key "a" as a file and refuse "a/b" beside it, which the blob store must keep.
//
// Each New starts a container of its own, and NewBucket makes it an empty bucket. The tests need Docker and run only
// with TEST_S3=container (scripts/task.sh go:test-blob; CI in the slow suite): without it New
// skips the test, and with it a Docker that cannot start the container fails the test.
package s3test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The environment variables of the tests; see the package documentation.
const (
	EnvS3    = "TEST_S3"
	EnvImage = "TEST_S3_IMAGE"
)

// DefaultImage is the container image of the tests, pinned by tag and digest.
const DefaultImage = "dxflrs/garage:v2.1.0@sha256:4c9b34c113e61358466e83fd6e7d66e6d18657ede14b776eb78a93ee8da7cf6a"

// The credentials and the region of the server.
const (
	AccessKeyID     = "GK0123456789abcdef01234567"
	SecretAccessKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	Region          = "garage"
)

// config is the configuration of the single-node server.
const config = `metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "00000000000000000000000000000000000000000000000000000000000000ff"

[s3_api]
s3_region = "` + Region + `"
api_bind_addr = "[::]:3900"
`

// Server is the S3-compatible server of one test.
type Server struct {
	// Endpoint is the http: URL of the S3 API; address the bucket in the path.
	Endpoint string

	c       testcontainers.Container
	buckets int
}

// New starts a server for t, whose key AccessKeyID may read and write the buckets of
// NewBucket, and stops it when t ends. It skips t unless TEST_S3 is "container".
func New(t testing.TB) *Server {
	t.Helper()
	if os.Getenv(EnvS3) != "container" {
		t.Skipf("the S3 server tests run with %s=container (scripts/task.sh go:test-blob)", EnvS3)
	}
	image := os.Getenv(EnvImage)
	if image == "" {
		image = DefaultImage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("3900/tcp"),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			Reader: strings.NewReader(config), ContainerFilePath: "/etc/garage.toml", FileMode: 0o600,
		}),
		// The server logs this once both of its listeners are up, after it wrote its node key;
		// the image has no shell for the port check inside the container.
		testcontainers.WithWaitStrategy(wait.ForLog("Listening on [::]:3901").WithStartupTimeout(time.Minute)),
	)
	if c != nil {
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	}
	if err != nil {
		t.Fatalf("s3test: start container %s: %v", image, err)
	}
	s := &Server{c: c}
	// One node holds every block: assign it a role in the layout and apply the layout, then
	// make the key.
	nodeID, _, _ := strings.Cut(lastLine(s.garage(t, "node", "id", "-q")), "@")
	s.garage(t, "layout", "assign", "-z", "dc1", "-c", "1G", nodeID)
	s.garage(t, "layout", "apply", "--version", "1")
	s.garage(t, "key", "import", "--yes", "-n", "test", AccessKeyID, SecretAccessKey)
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("s3test: container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "3900/tcp")
	if err != nil {
		t.Fatalf("s3test: port: %v", err)
	}
	s.Endpoint = "http://" + net.JoinHostPort(host, strconv.Itoa(int(port.Num())))
	return s
}

// NewBucket creates an empty bucket that the key AccessKeyID may read and write, and returns
// its name. It is not safe for concurrent use.
func (s *Server) NewBucket(t testing.TB) string {
	t.Helper()
	s.buckets++
	name := fmt.Sprintf("test-%d", s.buckets)
	s.garage(t, "bucket", "create", name)
	s.garage(t, "bucket", "allow", "--read", "--write", "--owner", name, "--key", "test")
	return name
}

// garage runs the command line tool of the server with args and returns its output.
func (s *Server) garage(t testing.TB, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	code, out, err := s.c.Exec(ctx, append([]string{"/garage"}, args...), tcexec.Multiplexed())
	var b bytes.Buffer
	if out != nil {
		_, _ = b.ReadFrom(out)
	}
	if err != nil || code != 0 {
		t.Fatalf("s3test: garage %s: exit %d, %v: %s", strings.Join(args, " "), code, err, b.String())
	}
	return b.String()
}

// lastLine returns the last line of s, past any log line.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
