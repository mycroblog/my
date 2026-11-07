// SPDX-License-Identifier: AGPL-3.0-only

package opensearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// Error is an error response of the engine. It holds the HTTP status and the type of the
// engine's error, such as index_not_found_exception, and never its reason, which can quote the
// document or the query.
type Error struct {
	Op     string
	Status int
	// Type is error.type of the response, empty when the response has none (a document that
	// is not there answers 404 with no error).
	Type string
}

func (e *Error) Error() string {
	if e.Type == "" {
		return fmt.Sprintf("opensearch: %s: HTTP %d", e.Op, e.Status)
	}
	return fmt.Sprintf("opensearch: %s: HTTP %d %s", e.Op, e.Status, e.Type)
}

// errorResponse is the part of an error response that the adapter reads.
type errorResponse struct {
	Error json.RawMessage `json:"error"`
}

// errorType returns error.type of an error response: an object in most responses, a string in a
// few (such as a 404 of an alias).
func errorType(b []byte) string {
	var r errorResponse
	if json.Unmarshal(b, &r) != nil || len(r.Error) == 0 {
		return ""
	}
	var obj struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(r.Error, &obj) == nil {
		return safeType(obj.Type)
	}
	return "error"
}

// safeType keeps an error type only when it looks like one (snake_case words), so that nothing
// of a proxy's own page reaches an error.
func safeType(t string) string {
	if t == "" || len(t) > 100 {
		return ""
	}
	for _, r := range t {
		if (r < 'a' || r > 'z') && r != '_' && (r < '0' || r > '9') {
			return ""
		}
	}
	return t
}

// maxResponseBytes bounds a response of the API that the adapter reads.
const maxResponseBytes = 4 << 20

// call sends method to path under the base URL with query and the JSON of body unless it is nil,
// and decodes the response into out unless it is nil. A response other than 2xx is an *Error.
// No error holds the URL's credentials (it has none), the headers, or the bodies.
func (x *Index) call(ctx context.Context, op, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("opensearch: %s: encode: %w", op, err)
		}
		payload = b
	}
	// path is escaped already: the names of the index are plain, and IDs go through url.PathEscape.
	target := x.base + "/" + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	r, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("opensearch: %s: %w", op, err)
	}
	r.Header.Set("Accept", "application/json")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	switch {
	case x.signer != nil:
		if err := x.signer.sign(ctx, r, payload); err != nil {
			return fmt.Errorf("opensearch: %s: %w", op, err)
		}
	case x.o.Username != "":
		r.SetBasicAuth(x.o.Username, x.o.Password)
	}
	resp, err := x.o.HTTPClient.Do(r)
	if err != nil {
		return fmt.Errorf("opensearch: %s: %w", op, transportError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("opensearch: %s: read the response: %w", op, transportError(err))
	}
	if resp.StatusCode/100 != 2 {
		return &Error{Op: op, Status: resp.StatusCode, Type: errorType(b)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("opensearch: %s: decode the response: %w", op, err)
	}
	return nil
}

// transportError keeps what failed of an error of the HTTP client, the context's error among
// them, and drops the URL that *url.Error quotes: it holds nothing secret, but the path of a
// write holds a status ID, which the logs do not need twice.
func transportError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// signer signs the requests of Amazon OpenSearch Service with Signature Version 4.
type signer struct {
	a AWSSigning
	s *v4.Signer
}

func newSigner(a AWSSigning) (*signer, error) {
	switch {
	case a.Credentials == nil:
		return nil, errors.New("opensearch: AWS request signing needs credentials")
	case a.Region == "":
		return nil, errors.New("opensearch: AWS request signing needs a region")
	case a.Service != ServiceManaged:
		return nil, fmt.Errorf("opensearch: AWS request signing supports the service %q only", ServiceManaged)
	}
	if a.Now == nil {
		a.Now = time.Now
	}
	return &signer{a: a, s: v4.NewSigner()}, nil
}

// sign signs r, whose body is payload. The hash of the payload is signed and sent in
// X-Amz-Content-Sha256 too.
func (s *signer) sign(ctx context.Context, r *http.Request, payload []byte) error {
	creds, err := s.a.Credentials.Retrieve(ctx)
	if err != nil {
		// The SDK's errors name where it looked for the credentials, never the credentials.
		return fmt.Errorf("retrieve the AWS credentials: %w", err)
	}
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	r.Header.Set("X-Amz-Content-Sha256", hash)
	if err := s.s.SignHTTP(ctx, creds, r, hash, s.a.Service, s.a.Region, s.a.Now().UTC()); err != nil {
		return errors.New("sign the request")
	}
	return nil
}
