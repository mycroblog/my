// SPDX-License-Identifier: AGPL-3.0-only

// Package gcpkmstest is an in-memory stand-in for the Encrypt and Decrypt methods of the Cloud
// KMS API, for the tests of the gcpkms key manager and of what is built on it. It serves gRPC
// in the process, keeps versions of symmetric CryptoKeys with their states, checks the CRC32C
// checksums of the requests, and fails with the status codes that Cloud KMS documents. It
// protects nothing; never use it outside tests.
package gcpkmstest

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"hash/crc32"
	"net"
	"strconv"
	"sync"
	"testing"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Server is the stand-in for Cloud KMS. Its methods are safe for concurrent use.
type Server struct {
	kmspb.UnimplementedKeyManagementServiceServer

	mu     sync.Mutex
	keys   map[string]*cryptoKey
	denied map[string]bool
	fail   []codes.Code
	calls  int
}

type cryptoKey struct {
	versions []*version
	primary  int
}

type version struct {
	aead    cipher.AEAD
	enabled bool
}

// Start serves a Server until the test ends and returns it with the client options that reach
// it without authentication. Every name of keys is a CryptoKey with one enabled version.
func Start(t testing.TB, keys ...string) (*Server, []option.ClientOption) {
	t.Helper()
	s := &Server{keys: map[string]*cryptoKey{}, denied: map[string]bool{}}
	for _, k := range keys {
		s.AddKey(k)
	}
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	kmspb.RegisterKeyManagementServiceServer(gs, s)
	go func() { _ = gs.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///gcpkmstest",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("gcpkmstest: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		gs.Stop()
	})
	return s, []option.ClientOption{option.WithGRPCConn(conn), option.WithoutAuthentication()}
}

// AddKey creates the CryptoKey name with one enabled version, unless it exists.
func (s *Server) AddKey(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys[name] == nil {
		s.keys[name] = &cryptoKey{versions: []*version{newVersion()}}
	}
}

// Rotate adds a version to the CryptoKey name and makes it the primary one, as a rotation does.
func (s *Server) Rotate(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.keys[name]
	k.versions = append(k.versions, newVersion())
	k.primary = len(k.versions) - 1
}

// DisableVersion disables version v (1 is the first) of the CryptoKey name.
func (s *Server) DisableVersion(name string, v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[name].versions[v-1].enabled = false
}

// Deny makes every call on the CryptoKey name fail with PERMISSION_DENIED.
func (s *Server) Deny(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.denied[name] = true
}

// FailNext makes the next calls fail with codes, one call per code.
func (s *Server) FailNext(c ...codes.Code) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = append(s.fail, c...)
}

// Calls returns how many Encrypt and Decrypt calls the Server received.
func (s *Server) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func newVersion() *version {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		panic(err)
	}
	return &version{aead: aead, enabled: true}
}

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

func checksum(b []byte) *wrapperspb.Int64Value {
	return wrapperspb.Int64(int64(crc32.Checksum(b, castagnoli)))
}

// matches reports whether an optional checksum of a request is absent or that of b.
func matches(sum *wrapperspb.Int64Value, b []byte) bool {
	return sum == nil || sum.GetValue() == checksum(b).GetValue()
}

// key returns the CryptoKey of a call, or its error. s.mu must be held.
func (s *Server) key(name string) (*cryptoKey, error) {
	s.calls++
	if len(s.fail) > 0 {
		c := s.fail[0]
		s.fail = s.fail[1:]
		return nil, status.Errorf(c, "gcpkmstest: injected %s", c)
	}
	if s.denied[name] {
		return nil, status.Errorf(codes.PermissionDenied, "Permission 'cloudkms.cryptoKeyVersions.useToEncrypt' denied on resource '%s' (or it may not exist).", name)
	}
	k := s.keys[name]
	if k == nil {
		return nil, status.Errorf(codes.NotFound, "%s not found.", name)
	}
	return k, nil
}

// Encrypt implements kmspb.KeyManagementServiceServer. The ciphertext is the index of the
// primary version followed by AES-256-GCM under that version.
func (s *Server) Encrypt(_ context.Context, req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, err := s.key(req.GetName())
	if err != nil {
		return nil, err
	}
	if !matches(req.GetPlaintextCrc32C(), req.GetPlaintext()) ||
		!matches(req.GetAdditionalAuthenticatedDataCrc32C(), req.GetAdditionalAuthenticatedData()) {
		return nil, status.Error(codes.InvalidArgument, "The checksum in field plaintext_crc32c did not match the data in field plaintext.")
	}
	v := k.versions[k.primary]
	if !v.enabled {
		return nil, status.Errorf(codes.FailedPrecondition, "%s/cryptoKeyVersions/%d is not enabled, current state is: DISABLED.", req.GetName(), k.primary+1)
	}
	ct := append([]byte{byte(k.primary)}, v.aead.Seal(nil, nil, req.GetPlaintext(), req.GetAdditionalAuthenticatedData())...) //nolint:gosec // G407 false positive: NewGCMWithRandomNonce generates the nonce and requires a nil one
	return &kmspb.EncryptResponse{
		Name:                    req.GetName() + "/cryptoKeyVersions/" + strconv.Itoa(k.primary+1),
		Ciphertext:              ct,
		CiphertextCrc32C:        checksum(ct),
		VerifiedPlaintextCrc32C: req.GetPlaintextCrc32C() != nil,
		VerifiedAdditionalAuthenticatedDataCrc32C: req.GetAdditionalAuthenticatedDataCrc32C() != nil,
		ProtectionLevel: kmspb.ProtectionLevel_SOFTWARE,
	}, nil
}

// Decrypt implements kmspb.KeyManagementServiceServer.
func (s *Server) Decrypt(_ context.Context, req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, err := s.key(req.GetName())
	if err != nil {
		return nil, err
	}
	if !matches(req.GetCiphertextCrc32C(), req.GetCiphertext()) ||
		!matches(req.GetAdditionalAuthenticatedDataCrc32C(), req.GetAdditionalAuthenticatedData()) {
		return nil, status.Error(codes.InvalidArgument, "The checksum in field ciphertext_crc32c did not match the data in field ciphertext.")
	}
	invalid := status.Error(codes.InvalidArgument, "Decryption failed: the ciphertext is invalid.")
	ct := req.GetCiphertext()
	if len(ct) < 1 || int(ct[0]) >= len(k.versions) {
		return nil, invalid
	}
	v := k.versions[ct[0]]
	if !v.enabled {
		return nil, status.Errorf(codes.FailedPrecondition, "%s/cryptoKeyVersions/%d is not enabled, current state is: DISABLED.", req.GetName(), int(ct[0])+1)
	}
	pt, err := v.aead.Open(nil, nil, ct[1:], req.GetAdditionalAuthenticatedData())
	if err != nil {
		return nil, invalid
	}
	return &kmspb.DecryptResponse{
		Plaintext:       pt,
		PlaintextCrc32C: checksum(pt),
		UsedPrimary:     int(ct[0]) == k.primary,
		ProtectionLevel: kmspb.ProtectionLevel_SOFTWARE,
	}, nil
}
