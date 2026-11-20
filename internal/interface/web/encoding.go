// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

// The build holds its text files only compressed (web/app/scripts/compress.ts): a file the server
// serves as <file>.br and <file>.gz, a file it only reads (the Vite manifest, the ETag list) as
// <file>.gz. The server answers with Brotli when the request accepts it, with gzip otherwise, and
// gunzips the .gz for a request that accepts neither; it needs no Brotli encoder or decoder.

// Content-Encoding values of the compressed forms, and the extensions of their files.
const (
	encodingBrotli = "br"
	encodingGzip   = "gzip"
	extBrotli      = ".br"
	extGzip        = ".gz"
)

// builtFile is a file of the build that the server serves, by the path it is served at.
type builtFile struct {
	// etag and size are those of the content; plain reports that the build holds it as it is.
	etag  string
	size  int64
	plain bool
	// brotli and gzip are the ETags of the compressed forms that the build holds, "" for none.
	brotli, gzip string
}

// compressed reports whether the build holds a compressed form of the file.
func (f builtFile) compressed() bool { return f.brotli != "" || f.gzip != "" }

// splitEncoding returns the path that a file of the build is served at and the Content-Encoding
// of its form: "" for a file held as it is.
func splitEncoding(p string) (name, encoding string) {
	switch {
	case strings.HasSuffix(p, extBrotli):
		return strings.TrimSuffix(p, extBrotli), encodingBrotli
	case strings.HasSuffix(p, extGzip):
		return strings.TrimSuffix(p, extGzip), encodingGzip
	}
	return p, ""
}

// readBuildFile reads a file of the build that the server reads itself: the file, or else its
// gzipped form, gunzipped. A file that the build holds in neither form is fs.ErrNotExist.
func readBuildFile(build fs.FS, name string) ([]byte, error) {
	data, err := fs.ReadFile(build, name)
	if !errors.Is(err, fs.ErrNotExist) {
		return data, err
	}
	f, gzErr := build.Open(name + extGzip)
	if errors.Is(gzErr, fs.ErrNotExist) {
		return nil, err
	}
	if gzErr != nil {
		return nil, gzErr
	}
	defer func() { _ = f.Close() }()
	return gunzip(f, -1)
}

// gunzip decompresses r; with size 0 or more, the content must be that long.
func gunzip(r io.Reader, size int64) ([]byte, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	limit := io.Reader(zr)
	if size >= 0 {
		buf.Grow(int(size))
		limit = io.LimitReader(zr, size+1)
	}
	if _, err := io.Copy(&buf, limit); err != nil {
		return nil, err
	}
	if size >= 0 && int64(buf.Len()) != size {
		return nil, fmt.Errorf("gunzipped to %d bytes, not the %d of the list", buf.Len(), size)
	}
	return buf.Bytes(), nil
}

// acceptedEncodings reports whether the request accepts Brotli and gzip by its Accept-Encoding
// (RFC 9110 §12.5.3): a coding is accepted when it is listed with a q above 0, or when it is not
// listed and * is. A request without Accept-Encoding accepts neither, so that it gets the content as
// it is.
func acceptedEncodings(r *http.Request) (brotli, gz bool) {
	q := map[string]float64{}
	for _, value := range r.Header.Values("Accept-Encoding") {
		for item := range strings.SplitSeq(value, ",") {
			coding, params, _ := strings.Cut(item, ";")
			coding = strings.ToLower(strings.TrimSpace(coding))
			if coding == "" {
				continue
			}
			if coding == "x-gzip" {
				coding = encodingGzip
			}
			weight := 1.0
			for param := range strings.SplitSeq(params, ";") {
				k, v, ok := strings.Cut(param, "=")
				if !ok || !strings.EqualFold(strings.TrimSpace(k), "q") {
					continue
				}
				if weight, ok = parseQ(strings.TrimSpace(v)); !ok {
					weight = 0
				}
			}
			q[coding] = weight
		}
	}
	accepts := func(coding string) bool {
		if w, ok := q[coding]; ok {
			return w > 0
		}
		return q["*"] > 0
	}
	return accepts(encodingBrotli), accepts(encodingGzip)
}

// parseQ parses a qvalue (RFC 9110 §12.4.2): 0 to 1, with at most three decimals.
func parseQ(s string) (float64, bool) {
	if len(s) == 0 || len(s) > 5 || (s[0] != '0' && s[0] != '1') {
		return 0, false
	}
	w, err := strconv.ParseFloat(s, 64)
	if err != nil || w > 1 {
		return 0, false
	}
	return w, true
}

// serveFile serves a file of the build with its ETag, answering conditional requests. A file
// that the build holds compressed is sent in the form the request accepts, with the ETag of that
// form, and Vary: Accept-Encoding.
func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, name, cacheControl string) {
	f := h.files[name]
	hd := w.Header()
	hd.Set("Cache-Control", cacheControl)
	if !f.compressed() {
		h.serveBuilt(w, r, name, name, f.etag)
		return
	}
	addVary(hd, "Accept-Encoding")
	if hd.Get("Content-Type") == "" {
		// http.ServeContent would sniff the compressed bytes.
		ctype := mime.TypeByExtension(path.Ext(name))
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		hd.Set("Content-Type", ctype)
	}
	brotli, gz := acceptedEncodings(r)
	switch {
	case brotli && f.brotli != "":
		h.serveBuilt(encodedWriter{w, encodingBrotli}, r, name, name+extBrotli, f.brotli)
	case gz && f.gzip != "":
		h.serveBuilt(encodedWriter{w, encodingGzip}, r, name, name+extGzip, f.gzip)
	case f.plain:
		h.serveBuilt(w, r, name, name, f.etag)
	default:
		zf, err := h.o.Build.Open(name + extGzip)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		defer func() { _ = zf.Close() }()
		content, err := gunzip(zf, f.size)
		if err != nil {
			h.fail(w, r, fmt.Errorf("web: gunzip %s: %w", name, err))
			return
		}
		hd.Set("ETag", f.etag)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
	}
}

// encodedWriter sets the Content-Encoding of a compressed form when the response is written:
// http.ServeContent leaves the Content-Length out of a response that has a Content-Encoding by then,
// and the length of a compressed form is known.
type encodedWriter struct {
	http.ResponseWriter
	encoding string
}

func (w encodedWriter) WriteHeader(code int) {
	if code == http.StatusOK || code == http.StatusPartialContent {
		w.Header().Set("Content-Encoding", w.encoding)
	}
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w encodedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// serveBuilt serves the file at file of the build as the response for name, with etag.
func (h *Handler) serveBuilt(w http.ResponseWriter, r *http.Request, name, file, etag string) {
	f, err := h.o.Build.Open(file)
	if err != nil {
		h.o.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	content, ok := f.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(f)
		if err != nil {
			h.o.NotFound(w, r)
			return
		}
		content = bytes.NewReader(data)
	}
	w.Header().Set("ETag", etag)
	http.ServeContent(w, r, name, time.Time{}, content)
}
