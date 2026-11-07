// SPDX-License-Identifier: AGPL-3.0-only

// Package googletest is an in-process stand-in for the TranslateText method of the Cloud Translation v3 API, for the
// tests of the google translator and of what is built on it. It serves gRPC in the process, translates by upper-casing
// the text of the HTML it receives and keeping its tags, detects every source as English, and fails with the status codes
// that Cloud Translation documents. Never use it outside tests.
package googletest

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/translate/apiv3/translatepb"
	"golang.org/x/net/html"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Server is the stand-in for Cloud Translation. Its methods are safe for concurrent use.
type Server struct {
	translatepb.UnimplementedTranslationServiceServer

	mu        sync.Mutex
	languages map[string]bool
	fail      []codes.Code
	requests  []*translatepb.TranslateTextRequest
	// rewrite, when set, replaces each translation of the stand-in.
	rewrite func(string) string
}

// Languages are the language codes that a Server translates unless Start names others.
var Languages = []string{"en", "ja", "fr", "de", "zh-CN", "zh-TW"}

// Start serves a Server until the test ends and returns it with the client options that reach it without
// authentication. It translates between the codes of languages, or of Languages when there are none.
func Start(t testing.TB, languages ...string) (*Server, []option.ClientOption) {
	t.Helper()
	if len(languages) == 0 {
		languages = Languages
	}
	s := &Server{languages: map[string]bool{}}
	for _, l := range languages {
		s.languages[l] = true
	}
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	translatepb.RegisterTranslationServiceServer(gs, s)
	go func() { _ = gs.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///googletest",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("googletest: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		gs.Stop()
	})
	return s, []option.ClientOption{option.WithGRPCConn(conn), option.WithoutAuthentication()}
}

// FailNext makes the next call fail with code, after the failures that are already queued.
func (s *Server) FailNext(code codes.Code) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = append(s.fail, code)
}

// Rewrite makes every later translation f of the stand-in's own, to answer what Google might.
func (s *Server) Rewrite(f func(string) string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rewrite = f
}

// Requests returns the requests that reached the stand-in, in order.
func (s *Server) Requests() []*translatepb.TranslateTextRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*translatepb.TranslateTextRequest(nil), s.requests...)
}

// TranslateText implements the method of Cloud Translation.
func (s *Server) TranslateText(_ context.Context, req *translatepb.TranslateTextRequest) (*translatepb.TranslateTextResponse, error) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	var fail codes.Code
	if len(s.fail) > 0 {
		fail, s.fail = s.fail[0], s.fail[1:]
	}
	rewrite := s.rewrite
	s.mu.Unlock()
	switch {
	case fail == codes.ResourceExhausted:
		return nil, status.Error(fail, "Quota exceeded for quota metric 'v3 general model characters'.")
	case fail != codes.OK:
		return nil, status.Error(fail, "The service is currently unavailable.")
	case !strings.HasPrefix(req.GetParent(), "projects/"):
		return nil, status.Error(codes.InvalidArgument, "Invalid resource name.")
	case len(req.GetContents()) == 0 || len(req.GetContents()) > 1024:
		return nil, status.Error(codes.InvalidArgument, "The number of contents is out of range.")
	case req.GetMimeType() != "text/html":
		return nil, status.Error(codes.InvalidArgument, "Unsupported mime type.")
	case !s.languages[req.GetTargetLanguageCode()]:
		return nil, invalidLanguage("target_language_code", "Target language is invalid.")
	case req.GetSourceLanguageCode() != "" && !s.languages[req.GetSourceLanguageCode()]:
		return nil, invalidLanguage("source_language_code", "Source language is invalid.")
	}
	detected := ""
	if req.GetSourceLanguageCode() == "" {
		detected = "en"
	}
	res := &translatepb.TranslateTextResponse{}
	for _, c := range req.GetContents() {
		tr := translateHTML(c)
		if rewrite != nil {
			tr = rewrite(tr)
		}
		res.Translations = append(res.Translations, &translatepb.Translation{TranslatedText: tr, DetectedLanguageCode: detected})
	}
	return res, nil
}

func invalidLanguage(field, message string) error {
	st, err := status.New(codes.InvalidArgument, message).WithDetails(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: field, Description: message}},
	})
	if err != nil {
		return status.Error(codes.InvalidArgument, message)
	}
	return st.Err()
}

// translateHTML upper-cases the text of an HTML content, keeps its tags, and leaves the content of an element of
// translate="no" or the class notranslate as it is, as Google does.
func translateHTML(content string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(content))
	untranslated := 0
	var stack []bool
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return b.String()
		}
		tok := z.Token()
		switch tt {
		case html.TextToken:
			if untranslated == 0 {
				tok.Data = strings.ToUpper(tok.Data)
			}
		case html.StartTagToken:
			if tok.Data != "br" {
				keep := notranslate(tok)
				stack = append(stack, keep)
				if keep {
					untranslated++
				}
			}
		case html.EndTagToken:
			if len(stack) > 0 {
				if stack[len(stack)-1] {
					untranslated--
				}
				stack = stack[:len(stack)-1]
			}
		}
		b.WriteString(tok.String())
	}
}

func notranslate(tok html.Token) bool {
	for _, a := range tok.Attr {
		if a.Key == "translate" && a.Val == "no" || a.Key == "class" && strings.Contains(" "+a.Val+" ", " notranslate ") {
			return true
		}
	}
	return false
}
