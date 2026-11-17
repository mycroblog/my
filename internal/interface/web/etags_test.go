// SPDX-License-Identifier: AGPL-3.0-only

package web_test

import (
	"encoding/json"
	"io/fs"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/mycroblog/my/internal/interface/web"
)

// openLog is a build that records the files opened in it. It is only an fs.FS, so that reading a
// file goes through Open.
type openLog struct {
	build fs.FS
	mu    sync.Mutex
	names []string
}

func (l *openLog) Open(name string) (fs.File, error) {
	l.mu.Lock()
	l.names = append(l.names, name)
	l.mu.Unlock()
	return l.build.Open(name)
}

func (l *openLog) opened() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.names)
}

// TestStartReadsNoServedFile checks that building the handler opens no file that it serves: the
// ETags come from the list of the web build, and a file is first read by the request for it.
func TestStartReadsNoServedFile(t *testing.T) {
	build := &openLog{build: fakeBuild()}
	_, mux := newHandler(t, web.Options{Build: build})
	for _, name := range build.opened() {
		if !strings.HasPrefix(name, ".vite/") && name != "." && name != "assets" && name != "licenses" {
			t.Errorf("New opened %s; only the manifest, the list of ETags, and directories may be opened at start", name)
		}
	}

	rec := get(mux, "/sw.js", nil)
	// The ETag of today's server for this content: the first 18 bytes of its SHA-256.
	if got, want := rec.Header().Get("ETag"), `"520vKCZwcnzvZAko0YBwEdND"`; rec.Code != http.StatusOK || got != want {
		t.Errorf("GET /sw.js = %d with ETag %s, want %s", rec.Code, got, want)
	}
	if !slices.Contains(build.opened(), "sw.js") {
		t.Error("GET /sw.js did not open the file")
	}
}

// TestListETags pins the list's format and the ETag of a content, which web/app/scripts/etags.test.ts
// pins to the same value for the script of the web build.
func TestListETags(t *testing.T) {
	list, err := web.ListETags(fstest.MapFS{
		".vite/manifest.json": {Data: []byte(`{}`)},
		".hidden":             {Data: []byte("x")},
		"index.html":          {Data: []byte("vite")},
		"auth.html":           {Data: []byte("vite")},
		"sw.js":               {Data: []byte("hello\n")},
		"assets/empty.css":    {},
		"assets/index.html":   {Data: []byte("hello\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]struct {
		Size int64  `json:"size"`
		ETag string `json:"etag"`
	}
	if err := json.Unmarshal(list, &got); err != nil {
		t.Fatalf("the list %s: %v", list, err)
	}
	want := map[string]struct {
		Size int64  `json:"size"`
		ETag string `json:"etag"`
	}{
		"sw.js":             {6, "WJG1tSLV3whtD_CxEPvZ0hu0"},
		"assets/empty.css":  {0, "47DEQpj8HBSa-_TImW-5JCeu"},
		"assets/index.html": {6, "WJG1tSLV3whtD_CxEPvZ0hu0"},
	}
	if !maps.Equal(got, want) {
		t.Errorf("list = %v, want %v", got, want)
	}
}

// TestStaleETagList refuses a build whose list is not the list of its files, rather than serve a
// file with the ETag of another content.
func TestStaleETagList(t *testing.T) {
	for name, change := range map[string]func(fstest.MapFS){
		"no list":              func(b fstest.MapFS) { delete(b, web.ETagsPath) },
		"not JSON":             func(b fstest.MapFS) { b[web.ETagsPath] = &fstest.MapFile{Data: []byte(`{`)} },
		"a file added":         func(b fstest.MapFS) { b["assets/new-HHHH.js"] = &fstest.MapFile{Data: []byte("export {}")} },
		"a file removed":       func(b fstest.MapFS) { delete(b, "icon.svg") },
		"a file of a new size": func(b fstest.MapFS) { b["sw.js"] = &fstest.MapFile{Data: []byte("self.skipWaiting()")} },
	} {
		build := fakeBuild()
		change(build)
		_, err := web.New(web.Options{Host: "social.example", Build: build})
		if err == nil || !strings.Contains(err.Error(), "web:build") {
			t.Errorf("%s: New = %v, want an error that names web:build", name, err)
		}
	}
	// Hidden files and Vite's HTML are not served, so the list does not name them.
	build := fakeBuild()
	build[".vite/other.json"] = &fstest.MapFile{Data: []byte(`{}`)}
	build["index.html"] = &fstest.MapFile{Data: []byte("<!doctype html><title>another</title>")}
	if _, err := web.New(web.Options{Host: "social.example", Build: build}); err != nil {
		t.Errorf("New after a change to files that are not served: %v", err)
	}
}
