// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The error reports of the web client (ADR-0103 §6): POST /client_errors turns the body into a
// web kernel.ErrorEvent for the ErrorReporter. The strict server would answer a body it cannot
// read with a 400 or a 422; this endpoint answers 204 to every body that passes the guard and
// tells nothing of what it kept, so it is written by hand, behind the same guard.

// clientErrorsPattern is the http.ServeMux pattern of reportClientError.
const clientErrorsPattern = "POST " + BasePath + "/client_errors"

// ClientErrorLimit bounds the reports from one client IP, signed in or not (ADR-0103 §6).
var ClientErrorLimit = kernel.Limit{Name: "api.client_errors", Max: 10, Window: time.Minute}

// Bounds of a report.
const (
	// maxClientErrorBody is the largest body that the endpoint reads.
	maxClientErrorBody = 16 << 10
	// maxClientErrorFrames bounds the frames of the stack that the event keeps.
	maxClientErrorFrames = 50
	// maxClientErrorMessage bounds the bytes of the message, as the scrubbing of ADR-0103 §3.
	maxClientErrorMessage = 1 << 10
	// maxClientErrorName bounds the bytes of the error's name.
	maxClientErrorName = 256
)

// clientErrorKinds are the values of ClientErrorReport.kind.
var clientErrorKinds = map[ClientErrorReportKind]bool{
	ClientErrorReportKindRouteError: true, ClientErrorReportKindBoundaryError: true,
	ClientErrorReportKindUncaught: true, ClientErrorReportKindUnhandledrejection: true,
}

// registerClientErrors registers POST /client_errors (operation reportClientError), which the
// strict server does not generate.
func (s *Server) registerClientErrors(r Router) {
	r.Handle(clientErrorsPattern, s.guard(clientErrorsPattern, http.HandlerFunc(s.serveClientError)))
}

// serveClientError serves POST /client_errors: always 204. It ignores a body that is not
// application/json, is over maxClientErrorBody, is not a report, or has no frame of the
// server's own scripts, and the reports of a client IP past ClientErrorLimit.
func (s *Server) serveClientError(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return
	}
	if r.ContentLength > maxClientErrorBody {
		// The rest of the body is not read; the connection is closed instead of drained.
		w.Header().Set("Connection", "close")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxClientErrorBody+1))
	if err != nil || len(body) > maxClientErrorBody {
		w.Header().Set("Connection", "close")
		return
	}
	if !s.takeClientError(r) {
		return
	}
	var rep ClientErrorReport
	if err := json.Unmarshal(body, &rep); err != nil || !clientErrorKinds[rep.Kind] {
		return
	}
	_, signedIn := ViewerFrom(r.Context())
	ev, ok := s.clientErrorEvent(rep, !signedIn)
	if !ok || s.o.ErrorReporter == nil {
		return
	}
	s.o.ErrorReporter.Report(r.Context(), ev)
}

// takeClientError takes one report of the request's client IP from ClientErrorLimit. Like the
// other limits it lets the report through without a RateLimiter or when the store fails.
func (s *Server) takeClientError(r *http.Request) bool {
	if s.o.RateLimiter == nil {
		return true
	}
	d, err := s.o.RateLimiter.Take(r.Context(), s.o.ClientErrorLimit, s.clientKey(r))
	if err != nil {
		s.o.Logger.WarnContext(r.Context(), "client api rate limit unavailable", slog.String("limit", s.o.ClientErrorLimit.Name), slog.Any("error", err))
		return true
	}
	return d.Allowed
}

// clientErrorEvent returns the web event of rep, or false when its stack has no frame of the
// server's own scripts (the error of a browser extension or of a script of another site).
func (s *Server) clientErrorEvent(rep ClientErrorReport, unauthenticated bool) (kernel.ErrorEvent, bool) {
	frames := parseClientStack(rep.Stack, s.ownOrigins())
	if len(frames) == 0 {
		return kernel.ErrorEvent{}, false
	}
	name := cleanText(rep.Name, maxClientErrorName)
	if name == "" {
		name = "Error"
	}
	message := normalizeClientMessage(cleanText(rep.Message, maxClientErrorBody))
	route := clientRoute(rep.Route)
	lines := make([]string, len(frames))
	for i, f := range frames {
		lines[i] = f.String()
	}
	ev := kernel.ErrorEvent{
		Source:          kernel.ErrorSourceWeb,
		Kind:            kernel.ErrorKindError,
		Type:            name,
		Message:         message,
		Stack:           strings.Join(lines, "\n"),
		Where:           "web " + route,
		Unauthenticated: unauthenticated,
		Fingerprint:     clientErrorFingerprint(name, message, route, frames[0].chunk()),
	}
	if rep.Release != nil && reClientRelease.MatchString(*rep.Release) {
		ev.Version = *rep.Release
	}
	return ev, true
}

// ownOrigins returns the origins whose scripts are the server's own: Origin, and WebOrigin
// when the web client is served from another origin.
func (s *Server) ownOrigins() []string {
	origins := []string{strings.TrimSuffix(s.o.Origin, "/")}
	if s.o.WebOrigin != "" {
		origins = append(origins, strings.TrimSuffix(s.o.WebOrigin, "/"))
	}
	return origins
}

// clientFrame is one frame of a browser stack, of a script of the server's own origin.
type clientFrame struct {
	// Function is the function's name as the browser shows it; empty for an anonymous one.
	Function string
	// Path is the script's path on the origin, without its query and fragment.
	Path         string
	Line, Column string
}

// String returns the frame in the V8 form, with the path instead of the URL.
func (f clientFrame) String() string {
	loc := f.Path + ":" + f.Line + ":" + f.Column
	if f.Function == "" {
		return "    at " + loc
	}
	return "    at " + f.Function + " (" + loc + ")"
}

// reChunkHash matches the content hash that the build adds to the name of a chunk
// (assets/<name>-<hash>.js).
var reChunkHash = regexp.MustCompile(`-[A-Za-z0-9_-]{8}(\.[A-Za-z0-9]+)$`)

// chunk returns the file name of the frame's script without its content hash, which changes
// with every release: "assets/StatusPage-Dk2l_9a3.js" is "StatusPage.js".
func (f clientFrame) chunk() string {
	return reChunkHash.ReplaceAllString(path.Base(f.Path), "$1")
}

// The two forms of a frame of Error.stack: V8 (Chrome, Edge, and Node) writes
// "    at fn (url:line:col)" or "    at url:line:col"; Firefox and Safari write
// "fn@url:line:col" or "@url:line:col".
var (
	reV8Frame    = regexp.MustCompile(`^\s*at (?:(.*?) \()?(\S+):(\d+):(\d+)\)?\s*$`)
	reGeckoFrame = regexp.MustCompile(`^\s*(.*)@(\S+):(\d+):(\d+)\s*$`)
)

// parseClientStack returns the frames of stack, in either form, whose script is on one of
// origins, at most maxClientErrorFrames. The other lines (the message that V8 puts first,
// frames of eval, of other origins, and of browser extensions) are left out.
func parseClientStack(stack string, origins []string) []clientFrame {
	var frames []clientFrame
	for line := range strings.SplitSeq(stack, "\n") {
		m := reV8Frame.FindStringSubmatch(line)
		if m == nil {
			m = reGeckoFrame.FindStringSubmatch(line)
		}
		if m == nil {
			continue
		}
		p, ok := ownScriptPath(m[2], origins)
		if !ok {
			continue
		}
		fn := strings.TrimSpace(m[1])
		if !utf8.ValidString(fn) || len(fn) > maxClientErrorName {
			fn = ""
		}
		frames = append(frames, clientFrame{Function: fn, Path: p, Line: m[3], Column: m[4]})
		if len(frames) == maxClientErrorFrames {
			break
		}
	}
	return frames
}

// ownScriptPath returns the path of the script at raw when raw is an http(s) URL on one of
// origins.
func ownScriptPath(raw string, origins []string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return "", false
	}
	origin := u.Scheme + "://" + u.Host
	for _, o := range origins {
		if o != "" && strings.EqualFold(o, origin) {
			return u.EscapedPath(), true
		}
	}
	return "", false
}

// reClientRoute matches a route ID of the web client's router: a path pattern such as
// "/_app/statuses/$id", or "__root__".
var reClientRoute = regexp.MustCompile(`^(?:__root__|/[A-Za-z0-9_$./()\[\]{}-]*)$`)

// clientRoute returns the route of a report, or "unknown" for one that is missing or is not a
// route ID: the route names the place of the event and must stay a finite name.
func clientRoute(route *string) string {
	if route == nil || len(*route) > 256 || !reClientRoute.MatchString(*route) {
		return "unknown"
	}
	return *route
}

// reClientRelease matches the version of a build.
var reClientRelease = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)

// cleanText returns s as valid UTF-8 without NUL, cut to at most n bytes on a rune boundary.
func cleanText(s string, n int) string {
	s = strings.ReplaceAll(strings.ToValidUTF8(s, "�"), "\x00", "")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// The parts of a message that differ between the occurrences of the same error, or that may
// identify a person, replaced as the scrubbing of ADR-0103 §3 does, in this order.
var clientMessageReplacements = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>]+`), "<url>"},
	{regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), "<email>"},
	{regexp.MustCompile(`@[A-Za-z0-9_]+@[A-Za-z0-9.-]+`), "<acct>"},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`), "<id>"},
	{regexp.MustCompile(`\b[0-7][0-9A-HJKMNP-TV-Z]{25}\b`), "<id>"},
	{regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`), "<ip>"},
	{regexp.MustCompile(`(?i)\b(?:[0-9a-f]{1,4}:){2,7}[0-9a-f]{1,4}\b`), "<ip>"},
	{regexp.MustCompile(`\d{10,}`), "<n>"},
}

// normalizeClientMessage replaces the URLs, email addresses, accounts, IDs, IP addresses, and
// long numbers of a message and cuts it to maxClientErrorMessage bytes.
//
// The ErrorReporter scrubs every event the same way (ADR-0103 §3, a later change); the web
// fingerprint needs the normalized message before the event reaches it, so this endpoint keeps
// its own copy of the rules.
func normalizeClientMessage(m string) string {
	for _, r := range clientMessageReplacements {
		m = r.re.ReplaceAllString(m, r.with)
	}
	return cleanText(m, maxClientErrorMessage)
}

// clientErrorFingerprint returns the fingerprint of a web event (ADR-0103 §6): the minified
// function names and the chunk hashes change with every release, so it is made of the error's
// name, the normalized message, the route, and the first own chunk's name without its hash:
// the first 32 hexadecimal digits of their SHA-256.
func clientErrorFingerprint(name, message, route, chunk string) string {
	h := sha256.New()
	for _, part := range []string{string(kernel.ErrorSourceWeb), name, message, route, chunk} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}
