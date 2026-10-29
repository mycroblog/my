// SPDX-License-Identifier: AGPL-3.0-only

// Package awskmstest is an in-memory stand-in for the DescribeKey, Encrypt, and Decrypt
// operations of the AWS KMS API, for the tests of the awskms key manager and of what is built on
// it. It serves the AWS JSON 1.1 protocol over HTTP in the process, keeps symmetric keys with
// their aliases and states, binds each ciphertext to its key and encryption context, and fails
// with the exceptions that AWS KMS documents. It checks no signature and protects nothing; never
// use it outside tests.
package awskmstest

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
)

// Region is the region of the keys of the tests and of the client configuration of Start.
const Region = "us-east-1"

// tagSize is the size of the key tag that starts a ciphertext, so that Decrypt finds the key
// that encrypted it, as a ciphertext blob of AWS KMS names its key.
const tagSize = 16

// Server is the stand-in for AWS KMS. Its methods are safe for concurrent use.
type Server struct {
	// URL is the endpoint of the server (aws.Config.BaseEndpoint, AWS_ENDPOINT_URL_KMS).
	URL string

	mu       sync.Mutex
	keys     map[string]*key // ARN -> key
	aliases  map[string]string
	denied   map[string]bool
	fail     []string
	calls    int
	describe int
}

type key struct {
	aead     cipher.AEAD
	disabled bool
	spec     string
}

// Start serves a Server until the test ends and returns it with an aws.Config that reaches it
// with static credentials and without the SDK's backoff between retries. Every ARN of keys is an
// enabled symmetric key.
func Start(t testing.TB, keys ...string) (*Server, aws.Config) {
	t.Helper()
	s := &Server{keys: map[string]*key{}, aliases: map[string]string{}, denied: map[string]bool{}}
	for _, k := range keys {
		s.AddKey(k)
	}
	hs := httptest.NewServer(s)
	t.Cleanup(hs.Close)
	s.URL = hs.URL
	return s, aws.Config{
		Region:       Region,
		BaseEndpoint: aws.String(hs.URL),
		HTTPClient:   hs.Client(),
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test", Source: "awskmstest"}, nil
		}),
		Retryer: func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) { o.Backoff = retry.BackoffDelayerFunc(noBackoff) })
		},
	}
}

func noBackoff(int, error) (time.Duration, error) { return 0, nil }

// AddKey creates the enabled symmetric key arn, unless it exists.
func (s *Server) AddKey(arn string) { s.addKey(arn, "SYMMETRIC_DEFAULT") }

// AddAsymmetricKey creates the key arn with the key spec RSA_2048, which cannot wrap.
func (s *Server) AddAsymmetricKey(arn string) { s.addKey(arn, "RSA_2048") }

func (s *Server) addKey(arn, spec string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.keys[arn]; ok {
		return
	}
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	block, err := aes.NewCipher(k)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		panic(err)
	}
	s.keys[arn] = &key{aead: aead, spec: spec}
}

// AddAlias points the alias name (alias/...) at the key arn.
func (s *Server) AddAlias(name, arn string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aliases[name] = arn
}

// Deny makes every call on the key arn fail with AccessDeniedException.
func (s *Server) Deny(arn string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.denied[arn] = true
}

// Disable disables the key arn: Encrypt and Decrypt fail with DisabledException.
func (s *Server) Disable(arn string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[arn].disabled = true
}

// FailNext makes the next call fail with the exception named code, such as
// "ThrottlingException" or "KMSInternalException".
func (s *Server) FailNext(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = append(s.fail, code)
}

// Calls returns how many Encrypt and Decrypt requests reached the server.
func (s *Server) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// DescribeCalls returns how many DescribeKey requests reached the server.
func (s *Server) DescribeCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.describe
}

type request struct {
	KeyID               string            `json:"KeyId"`
	Plaintext           []byte            `json:"Plaintext"`
	CiphertextBlob      []byte            `json:"CiphertextBlob"`
	EncryptionContext   map[string]string `json:"EncryptionContext"`
	EncryptionAlgorithm string            `json:"EncryptionAlgorithm"`
}

// ServeHTTP implements the AWS JSON 1.1 protocol of AWS KMS.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	op, ok := strings.CutPrefix(r.Header.Get("X-Amz-Target"), "TrentService.")
	if r.Method != http.MethodPost || !ok {
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "unknown operation")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		writeError(w, http.StatusBadRequest, "MissingAuthenticationToken", "the request is not signed")
		return
	}
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "SerializationException", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if op == "DescribeKey" {
		s.describe++
	} else {
		s.calls++
	}
	if len(s.fail) > 0 {
		code := s.fail[0]
		s.fail = s.fail[1:]
		status := http.StatusBadRequest
		if code == "KMSInternalException" {
			status = http.StatusInternalServerError
		}
		writeError(w, status, code, "injected failure")
		return
	}
	switch op {
	case "DescribeKey":
		s.describeKey(w, req)
	case "Encrypt":
		s.encrypt(w, req)
	case "Decrypt":
		s.decrypt(w, req)
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "unknown operation "+op)
	}
}

// resolve returns the ARN of the key that id names: the key ARN, the key ID, an alias name, or
// an alias ARN.
func (s *Server) resolve(id string) (string, bool) {
	if _, ok := s.keys[id]; ok {
		return id, true
	}
	if i := strings.Index(id, ":alias/"); i >= 0 {
		id = id[i+1:]
	}
	if arn, ok := s.aliases[id]; ok {
		return arn, true
	}
	for arn := range s.keys {
		if strings.HasSuffix(arn, ":key/"+id) {
			return arn, true
		}
	}
	return "", false
}

// usable returns the key arn names, or writes the error of a key that cannot be used.
func (s *Server) usable(w http.ResponseWriter, id string) (string, *key, bool) {
	arn, ok := s.resolve(id)
	if !ok {
		writeError(w, http.StatusBadRequest, "NotFoundException", "key "+id+" does not exist")
		return "", nil, false
	}
	if s.denied[arn] {
		writeError(w, http.StatusBadRequest, "AccessDeniedException", "not authorized to use "+arn)
		return "", nil, false
	}
	k := s.keys[arn]
	if k.disabled {
		writeError(w, http.StatusBadRequest, "DisabledException", arn+" is disabled")
		return "", nil, false
	}
	if k.spec != "SYMMETRIC_DEFAULT" {
		writeError(w, http.StatusBadRequest, "InvalidKeyUsageException", arn+" is not for symmetric encryption")
		return "", nil, false
	}
	return arn, k, true
}

func (s *Server) describeKey(w http.ResponseWriter, req request) {
	arn, ok := s.resolve(req.KeyID)
	if !ok {
		writeError(w, http.StatusBadRequest, "NotFoundException", "key "+req.KeyID+" does not exist")
		return
	}
	if s.denied[arn] {
		writeError(w, http.StatusBadRequest, "AccessDeniedException", "not authorized to describe "+arn)
		return
	}
	k := s.keys[arn]
	state, usage := "Enabled", "ENCRYPT_DECRYPT"
	if k.disabled {
		state = "Disabled"
	}
	if k.spec != "SYMMETRIC_DEFAULT" {
		usage = "SIGN_VERIFY"
	}
	writeJSON(w, map[string]any{"KeyMetadata": map[string]any{
		"Arn": arn, "KeyId": arn[strings.LastIndex(arn, "/")+1:], "KeyState": state, "Enabled": !k.disabled,
		"KeySpec": k.spec, "KeyUsage": usage, "KeyManager": "CUSTOMER",
	}})
}

func tag(arn string) []byte {
	h := sha256.Sum256([]byte(arn))
	return h[:tagSize]
}

// contextAAD is the canonical form of an encryption context that the ciphertext authenticates.
func contextAAD(arn string, ec map[string]string) []byte {
	keys := make([]string, 0, len(ec))
	for k := range ec {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	b, _ := json.Marshal(struct {
		Key     string
		Context [][2]string
	}{arn, func() [][2]string {
		out := make([][2]string, 0, len(keys))
		for _, k := range keys {
			out = append(out, [2]string{k, ec[k]})
		}
		return out
	}()})
	return b
}

func (s *Server) encrypt(w http.ResponseWriter, req request) {
	arn, k, ok := s.usable(w, req.KeyID)
	if !ok {
		return
	}
	if len(req.Plaintext) == 0 || len(req.Plaintext) > 4096 {
		writeError(w, http.StatusBadRequest, "ValidationException", "the plaintext must be 1 to 4096 bytes")
		return
	}
	blob := append(tag(arn), k.aead.Seal(nil, nil, req.Plaintext, contextAAD(arn, req.EncryptionContext))...) //nolint:gosec // G407 false positive: NewGCMWithRandomNonce generates the nonce and requires a nil one
	writeJSON(w, map[string]any{"CiphertextBlob": blob, "KeyId": arn, "EncryptionAlgorithm": "SYMMETRIC_DEFAULT"})
}

func (s *Server) decrypt(w http.ResponseWriter, req request) {
	if len(req.CiphertextBlob) < tagSize {
		writeError(w, http.StatusBadRequest, "InvalidCiphertextException", "the ciphertext is malformed")
		return
	}
	var owner string
	for arn := range s.keys {
		if string(tag(arn)) == string(req.CiphertextBlob[:tagSize]) {
			owner = arn
		}
	}
	if owner == "" {
		writeError(w, http.StatusBadRequest, "InvalidCiphertextException", "the ciphertext names no key")
		return
	}
	id := req.KeyID
	if id == "" {
		id = owner
	}
	arn, k, ok := s.usable(w, id)
	if !ok {
		return
	}
	if arn != owner {
		writeError(w, http.StatusBadRequest, "IncorrectKeyException", "the ciphertext was not encrypted with "+arn)
		return
	}
	plain, err := k.aead.Open(nil, nil, req.CiphertextBlob[tagSize:], contextAAD(arn, req.EncryptionContext))
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidCiphertextException", "the ciphertext or the encryption context does not authenticate")
		return
	}
	writeJSON(w, map[string]any{"Plaintext": plain, "KeyId": arn, "EncryptionAlgorithm": "SYMMETRIC_DEFAULT"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("X-Amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
}
