// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

// HTTPClientNetwork is the test network that HTTPClientContract gives the client under test.
// The adapter must honor every field; production code never sets them.
type HTTPClientNetwork struct {
	// LookupNetIP is a stub DNS resolver the client uses instead of the system's.
	LookupNetIP func(ctx context.Context, host string) ([]netip.Addr, error)
	// Exempt lists the addresses of the contract's "public" test servers. They listen on
	// loopback, so the client treats exactly these address and port pairs as public while
	// keeping every other check, including the one on other ports of loopback.
	Exempt []netip.AddrPort
	// Ports are the ports the client allows instead of 443 and 80.
	Ports []int
	// AllowHTTP permits the http scheme, which the test servers use except for the TLS one.
	AllowHTTP bool
	// RootCAs trusts the certificate of the TLS test server.
	RootCAs *x509.CertPool
}

// HTTPClientContract verifies a kernel.HTTPClient adapter against the SSRF guard of
// ADR-0019 §1: it refuses every denied address range (as a URL literal and as the answer of
// the stub DNS), IPv4-mapped, NAT64, and 6to4 addresses that embed a denied IPv4 address,
// IPv4 addresses in decimal, octal, and hexadecimal notation, internal host names, other
// schemes and ports, redirects towards an internal server or from https to http, more than
// kernel.MaxRedirects redirects, and a stub DNS that rebinds a name to an internal address. It
// returns the 3xx answer of a request with NoRedirects, and asks a request's CheckRedirect
// before following each redirect hop. It
// enforces the body limit on the decompressed size and on Content-Length, and sends a
// User-Agent. The contract never uses the network: every server is an httptest server.
func HTTPClientContract(t *testing.T, newClient func(t *testing.T, n HTTPClientNetwork) kernel.HTTPClient) {
	t.Helper()
	t.Run("Fetches", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		var ua string
		e.public.handle("/doc", func(w http.ResponseWriter, r *http.Request) {
			ua = r.Header.Get("User-Agent")
			w.Header().Set("Content-Type", "application/activity+json")
			_, _ = w.Write([]byte(`{"id":"x"}`))
		})
		resp, err := e.get(t, e.public.url("public.example", "/doc"), kernel.HTTPPurposeJSON)
		if err != nil {
			t.Fatalf("Do() = %v", err)
		}
		body, err := resp.ReadAll()
		if err != nil || string(body) != `{"id":"x"}` || resp.StatusCode != http.StatusOK {
			t.Errorf("response = %d %q, %v", resp.StatusCode, body, err)
		}
		if got := resp.Header.Get("Content-Type"); got != "application/activity+json" {
			t.Errorf("Content-Type = %q", got)
		}
		if ua == "" || strings.HasPrefix(ua, "Go-http-client") {
			t.Errorf("User-Agent = %q, want the server's own", ua)
		}
	})
	t.Run("FollowsRedirectsAndReportsTheFinalURL", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		e.public.redirectChain("/hop", kernel.MaxRedirects, "/end")
		e.public.handle("/end", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("end")) })
		resp, err := e.get(t, e.public.url("public.example", "/hop0"), kernel.HTTPPurposeJSON)
		if err != nil {
			t.Fatalf("Do() after %d redirects = %v", kernel.MaxRedirects, err)
		}
		_, _ = resp.ReadAll()
		if want := e.public.url("public.example", "/end"); resp.URL != want {
			t.Errorf("URL = %q, want %q", resp.URL, want)
		}
	})
	t.Run("RefusesTooManyRedirects", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		e.public.redirectChain("/hop", kernel.MaxRedirects+1, "/end")
		e.public.handle("/end", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("end")) })
		_, err := e.get(t, e.public.url("public.example", "/hop0"), kernel.HTTPPurposeJSON)
		if !errors.Is(err, kernel.ErrTooManyRedirects) {
			t.Errorf("Do() after %d redirects = %v, want ErrTooManyRedirects", kernel.MaxRedirects+1, err)
		}
	})
	t.Run("ReturnsTheRedirectWithNoRedirects", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		var landed atomic.Int32
		e.public.handle("/inbox", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/landing", http.StatusFound)
		})
		e.public.handle("/landing", func(w http.ResponseWriter, _ *http.Request) {
			landed.Add(1)
			_, _ = w.Write([]byte("landing"))
		})
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			resp, err := e.client.Do(t.Context(), &kernel.HTTPRequest{
				Method: method, URL: e.public.url("public.example", "/inbox"), Body: []byte(`{}`),
				Purpose: kernel.HTTPPurposeJSON, NoRedirects: true,
			})
			if err != nil {
				t.Fatalf("Do(%s, NoRedirects) = %v", method, err)
			}
			_, _ = resp.ReadAll()
			if resp.StatusCode != http.StatusFound {
				t.Errorf("Do(%s, NoRedirects) status = %d, want the 302 itself", method, resp.StatusCode)
			}
		}
		if n := landed.Load(); n != 0 {
			t.Errorf("the redirect target got %d requests, want none", n)
		}
	})
	t.Run("ChecksEveryRedirectHop", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		e.dns["refused.example"] = []netip.Addr{e.public.addr.Addr()}
		var refusedHits atomic.Int32
		e.public.handle("/hop", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, e.public.url("public.example", "/hop2"), http.StatusFound)
		})
		e.public.handle("/hop2", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.Host, "refused.example") {
				refusedHits.Add(1)
				_, _ = w.Write([]byte("refused"))
				return
			}
			http.Redirect(w, r, e.public.url("refused.example", "/hop2"), http.StatusFound)
		})
		refusal := errors.New("refused by the caller")
		var seen []string
		resp, err := e.client.Do(t.Context(), &kernel.HTTPRequest{
			URL: e.public.url("public.example", "/hop"), Purpose: kernel.HTTPPurposeJSON,
			CheckRedirect: func(_ context.Context, u string) error {
				seen = append(seen, u)
				if strings.Contains(u, "refused.example") {
					return refusal
				}
				return nil
			},
		})
		if err == nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, refusal) {
			t.Errorf("Do() = %v, want the error of CheckRedirect", err)
		}
		want := []string{e.public.url("public.example", "/hop2"), e.public.url("refused.example", "/hop2")}
		if fmt.Sprint(seen) != fmt.Sprint(want) {
			t.Errorf("CheckRedirect saw %q, want %q", seen, want)
		}
		if n := refusedHits.Load(); n != 0 {
			t.Errorf("the refused host got %d requests, want none", n)
		}
	})
	t.Run("RefusesDeniedRanges", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		for _, a := range deniedAddrs {
			addr := netip.MustParseAddr(a)
			host := addr.String()
			if addr.Is6() {
				host = "[" + host + "]"
			}
			literal := "http://" + host + ":" + e.internal.port + "/"
			e.expectForbidden(t, literal)
			name := "a" + strconv.Itoa(len(e.dns)) + ".resolves.example"
			e.dns[name] = []netip.Addr{addr}
			e.expectForbidden(t, "http://"+name+":"+e.internal.port+"/")
		}
		if n := e.internal.hits.Load(); n != 0 {
			t.Errorf("the internal server got %d requests", n)
		}
	})
	t.Run("RefusesDecimalAndOtherIPv4Notations", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		for _, host := range []string{
			"2130706433",          // 127.0.0.1
			"0x7f000001",          // 127.0.0.1
			"017700000001",        // 127.0.0.1
			"0x7f.1",              // 127.0.0.1
			"127.1",               // 127.0.0.1
			"2852039166",          // 169.254.169.254
			"0xa9.0xfe.0xa9.0xfe", // 169.254.169.254
			"167772161",           // 10.0.0.1
		} {
			e.expectForbiddenOrInvalid(t, "http://"+host+":"+e.internal.port+"/")
		}
		if n := e.internal.hits.Load(); n != 0 {
			t.Errorf("the internal server got %d requests", n)
		}
	})
	t.Run("RefusesInternalHostNames", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		for _, name := range []string{"localhost", "metadata", "metadata.google.internal", "db.internal", "printer.local", "app.localhost", "intranet", "LOCALHOST", "localhost."} {
			// The stub DNS answers with the public server, so only the name can be the reason to refuse.
			e.dns[strings.TrimSuffix(strings.ToLower(name), ".")] = []netip.Addr{e.public.addr.Addr()}
			e.expectForbidden(t, "http://"+name+":"+e.public.port+"/")
		}
		if n := e.public.hits.Load(); n != 0 {
			t.Errorf("the public server got %d requests for internal names", n)
		}
	})
	t.Run("RefusesOtherSchemesAndPorts", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		for _, u := range []string{
			"ftp://public.example:" + e.public.port + "/",
			"file:///etc/passwd",
			"gopher://public.example:" + e.public.port + "/",
			"http://public.example:22/",
			"http://public.example:6379/",
		} {
			e.expectForbidden(t, u)
		}
	})
	t.Run("RefusesRedirectsToInternalDestinations", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		e.dns["internal.example"] = []netip.Addr{netip.MustParseAddr("10.0.0.1")}
		targets := []string{
			"http://127.0.0.1:" + e.internal.port + "/",
			"http://[::ffff:127.0.0.1]:" + e.internal.port + "/",
			"http://2130706433:" + e.internal.port + "/",
			"http://localhost:" + e.internal.port + "/",
			"http://metadata.google.internal:" + e.public.port + "/computeMetadata/v1/",
			"http://internal.example:" + e.public.port + "/",
			"http://169.254.169.254:" + e.public.port + "/latest/meta-data/",
		}
		for i, target := range targets {
			e.public.handle("/r"+strconv.Itoa(i), func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			})
			e.expectForbiddenOrInvalid(t, e.public.url("public.example", "/r"+strconv.Itoa(i)))
		}
		if n := e.internal.hits.Load(); n != 0 {
			t.Errorf("the internal server got %d requests", n)
		}
	})
	t.Run("RefusesRedirectsFromHTTPSToHTTP", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		e.public.handle("/plain", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("plain")) })
		e.secure.handle("/downgrade", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, e.public.url("public.example", "/plain"), http.StatusFound)
		})
		e.secure.handle("/ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
		resp, err := e.get(t, e.secure.url("example.com", "/ok"), kernel.HTTPPurposeJSON)
		if err != nil {
			t.Fatalf("Do(https) = %v", err)
		}
		_, _ = resp.ReadAll()
		e.expectForbidden(t, e.secure.url("example.com", "/downgrade"))
		if n := e.public.hits.Load(); n != 0 {
			t.Errorf("the http server got %d requests after a downgrade", n)
		}
	})
	t.Run("RefusesARebindingName", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		e.public.handle("/", func(w http.ResponseWriter, _ *http.Request) {
			// Closing the connection makes the next request resolve and connect again.
			w.Header().Set("Connection", "close")
			_, _ = w.Write([]byte("ok"))
		})
		var lookups atomic.Int32
		e.lookup = func(host string) []netip.Addr {
			if host != "rebind.example" {
				return nil
			}
			// The first answer is public; every later one (TTL 0) is internal.
			if lookups.Add(1) == 1 {
				return []netip.Addr{e.public.addr.Addr()}
			}
			return []netip.Addr{netip.MustParseAddr("10.1.2.3"), netip.MustParseAddr("169.254.169.254"), netip.MustParseAddr("127.0.0.2")}
		}
		u := e.public.url("rebind.example", "/")
		resp, err := e.get(t, u, kernel.HTTPPurposeJSON)
		if err != nil {
			t.Fatalf("first Do() = %v", err)
		}
		_, _ = resp.ReadAll()
		// A client that checked the name once, or trusted a cached answer, would now connect
		// to an internal address.
		e.expectForbidden(t, u)
		if n := lookups.Load(); n < 2 {
			t.Errorf("resolved the name %d times; the second request must resolve it again", n)
		}
	})
	t.Run("LimitsTheBody", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		limits, _ := kernel.DefaultHTTPLimits(kernel.HTTPPurposeJSON)
		limit := int(limits.MaxBodyBytes)
		e.public.handle("/exact", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(bytes.Repeat([]byte("a"), limit)) })
		e.public.handle("/over", func(w http.ResponseWriter, _ *http.Request) {
			// Chunked, without Content-Length, so only reading can find the size.
			w.(http.Flusher).Flush()
			_, _ = w.Write(bytes.Repeat([]byte("a"), limit+1))
		})
		var bombServed atomic.Bool
		e.public.handle("/bomb", func(w http.ResponseWriter, _ *http.Request) {
			// Recorded before writing: the client may give up and return before the write does.
			bombServed.Store(true)
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			_, _ = gz.Write(make([]byte, 4*limit))
			_ = gz.Close()
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
			_, _ = w.Write(buf.Bytes())
		})
		e.public.handle("/declared", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(limit+1))
			w.WriteHeader(http.StatusOK)
			// The body is never sent: the client must give up on the header alone.
		})

		resp, err := e.get(t, e.public.url("public.example", "/exact"), kernel.HTTPPurposeJSON)
		if err != nil {
			t.Fatalf("Do(exact) = %v", err)
		}
		if b, err := resp.ReadAll(); err != nil || len(b) != limit {
			t.Errorf("read %d bytes, %v; want %d", len(b), err, limit)
		}
		for _, path := range []string{"/over", "/bomb", "/declared"} {
			resp, err := e.get(t, e.public.url("public.example", path), kernel.HTTPPurposeJSON)
			if err == nil {
				_, err = resp.ReadAll()
			}
			if !errors.Is(err, kernel.ErrResponseTooLarge) {
				t.Errorf("%s: err = %v, want ErrResponseTooLarge", path, err)
			}
		}
		if !bombServed.Load() {
			t.Error("the compressed response was never requested")
		}
	})
	t.Run("LimitsTheBodyPerPurpose", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		// The CAPTCHA check allows a much smaller body than documents (ADR-0042 §3).
		limits, ok := kernel.DefaultHTTPLimits(kernel.HTTPPurposeHumanCheck)
		if !ok {
			t.Fatal("DefaultHTTPLimits(HTTPPurposeHumanCheck) is unknown")
		}
		limit := int(limits.MaxBodyBytes)
		e.public.handle("/exact", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(bytes.Repeat([]byte("a"), limit)) })
		e.public.handle("/over", func(w http.ResponseWriter, _ *http.Request) {
			w.(http.Flusher).Flush()
			_, _ = w.Write(bytes.Repeat([]byte("a"), limit+1))
		})
		resp, err := e.get(t, e.public.url("public.example", "/exact"), kernel.HTTPPurposeHumanCheck)
		if err != nil {
			t.Fatalf("Do(exact) = %v", err)
		}
		if b, err := resp.ReadAll(); err != nil || len(b) != limit {
			t.Errorf("read %d bytes, %v; want %d", len(b), err, limit)
		}
		resp, err = e.get(t, e.public.url("public.example", "/over"), kernel.HTTPPurposeHumanCheck)
		if err == nil {
			_, err = resp.ReadAll()
		}
		if !errors.Is(err, kernel.ErrResponseTooLarge) {
			t.Errorf("over the human check limit: err = %v, want ErrResponseTooLarge", err)
		}
	})
	t.Run("RefusesAnUnknownPurpose", func(t *testing.T) {
		e := newHTTPClientEnv(t, newClient)
		_, err := e.get(t, e.public.url("public.example", "/"), 0)
		if !errors.Is(err, kernel.ErrInvalidHTTPRequest) {
			t.Errorf("Do(no purpose) = %v, want ErrInvalidHTTPRequest", err)
		}
	})
}

// deniedAddrs has one address in each range that ADR-0019 §1 denies, and IPv6 addresses
// that embed a denied IPv4 address.
var deniedAddrs = []string{
	"0.0.0.1", "127.0.0.1", "127.255.255.254", "10.0.0.1", "172.16.0.1", "172.31.255.254", "192.168.1.1",
	"100.64.0.1", "100.100.100.200", "169.254.169.254", "192.0.0.8", "192.0.2.1", "198.18.0.1", "198.51.100.1",
	"203.0.113.1", "192.88.99.1", "224.0.0.1", "239.255.255.250", "240.0.0.1", "255.255.255.255",
	"::", "::1", "fc00::1", "fd00:ec2::254", "fe80::1", "fec0::1", "2001::1", "2001:db8::1", "100::1", "ff02::1",
	"::ffff:127.0.0.1", "::ffff:169.254.169.254", "::ffff:10.0.0.1",
	"64:ff9b::7f00:1", "64:ff9b::a9fe:a9fe", "64:ff9b:1:7f00:0:100::", "2002:7f00:1::1", "2002:a9fe:a9fe::1",
	"::127.0.0.1",
}

type httpClientEnv struct {
	client   kernel.HTTPClient
	public   *contractServer
	internal *contractServer
	secure   *contractServer

	mu     sync.Mutex
	dns    map[string][]netip.Addr
	lookup func(host string) []netip.Addr
}

func newHTTPClientEnv(t *testing.T, newClient func(t *testing.T, n HTTPClientNetwork) kernel.HTTPClient) *httpClientEnv {
	t.Helper()
	e := &httpClientEnv{
		public:   newContractServer(t, false),
		internal: newContractServer(t, false),
		secure:   newContractServer(t, true),
		dns:      map[string][]netip.Addr{},
	}
	e.dns["public.example"] = []netip.Addr{e.public.addr.Addr()}
	e.dns["example.com"] = []netip.Addr{e.secure.addr.Addr()}
	pool := x509.NewCertPool()
	pool.AddCert(e.secure.srv.Certificate())
	e.client = newClient(t, HTTPClientNetwork{
		LookupNetIP: e.resolve,
		Exempt:      []netip.AddrPort{e.public.addr, e.secure.addr},
		Ports:       []int{int(e.public.addr.Port()), int(e.internal.addr.Port()), int(e.secure.addr.Port())},
		AllowHTTP:   true,
		RootCAs:     pool,
	})
	return e
}

func (e *httpClientEnv) resolve(_ context.Context, host string) ([]netip.Addr, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lookup != nil {
		if a := e.lookup(host); a != nil {
			return a, nil
		}
	}
	if a, ok := e.dns[strings.TrimSuffix(strings.ToLower(host), ".")]; ok {
		return a, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func (e *httpClientEnv) get(t *testing.T, u string, p kernel.HTTPPurpose) (*kernel.HTTPResponse, error) {
	t.Helper()
	return e.client.Do(t.Context(), &kernel.HTTPRequest{URL: u, Purpose: p})
}

func (e *httpClientEnv) expectForbidden(t *testing.T, u string) {
	t.Helper()
	resp, err := e.get(t, u, kernel.HTTPPurposeJSON)
	if err == nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, kernel.ErrForbiddenDestination) {
		t.Errorf("Do(%s) = %v, want ErrForbiddenDestination", u, err)
	}
}

// expectForbiddenOrInvalid accepts ErrInvalidHTTPRequest too, for notations an adapter may
// reject as malformed instead of normalizing.
func (e *httpClientEnv) expectForbiddenOrInvalid(t *testing.T, u string) {
	t.Helper()
	resp, err := e.get(t, u, kernel.HTTPPurposeJSON)
	if err == nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, kernel.ErrForbiddenDestination) && !errors.Is(err, kernel.ErrInvalidHTTPRequest) {
		t.Errorf("Do(%s) = %v, want ErrForbiddenDestination", u, err)
	}
}

// contractServer is an httptest server on loopback that counts its requests.
type contractServer struct {
	srv    *httptest.Server
	mux    *http.ServeMux
	addr   netip.AddrPort
	port   string
	scheme string
	hits   atomic.Int32
}

func newContractServer(t *testing.T, tls bool) *contractServer {
	t.Helper()
	s := &contractServer{mux: http.NewServeMux(), scheme: "http"}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.mux.ServeHTTP(w, r)
	})
	if tls {
		s.srv, s.scheme = httptest.NewTLSServer(h), "https"
	} else {
		s.srv = httptest.NewServer(h)
	}
	t.Cleanup(s.srv.Close)
	u, err := url.Parse(s.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if s.addr, err = netip.ParseAddrPort(u.Host); err != nil {
		t.Fatal(err)
	}
	s.port = u.Port()
	return s
}

func (s *contractServer) handle(pattern string, h http.HandlerFunc) {
	s.mux.HandleFunc(pattern, h)
}

func (s *contractServer) url(host, path string) string {
	return s.scheme + "://" + host + ":" + s.port + path
}

// redirectChain makes prefix0 … prefix(n-1) redirect to the next one and the last to end.
func (s *contractServer) redirectChain(prefix string, n int, end string) {
	for i := range n {
		next := fmt.Sprintf("%s%d", prefix, i+1)
		if i == n-1 {
			next = end
		}
		s.handle(fmt.Sprintf("%s%d", prefix, i), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, next, http.StatusFound)
		})
	}
}
