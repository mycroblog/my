// SPDX-License-Identifier: AGPL-3.0-only

package s3_test

import (
	"crypto/md5" //nolint:gosec // the ETag of the fake, as S3 computes it
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is an in-memory S3 server of the path-style API that the Store uses: PutObject,
// GetObject (with a Range from a position and If-Match), HeadObject, DeleteObject, and the
// multipart uploads. It lets the contract and the unit tests run without a container.
type fakeS3 struct {
	bucket string

	mu       sync.Mutex
	objects  map[string]fakeObject
	uploads  map[string]*fakeUpload
	nextID   int
	aborted  int
	requests []string // "METHOD key?query" of every request
}

type fakeObject struct {
	data        []byte
	contentType string
	etag        string
	modTime     time.Time
}

type fakeUpload struct {
	key         string
	contentType string
	parts       map[int][]byte
}

func newFakeS3(t *testing.T, bucket string) (*fakeS3, *httptest.Server) {
	t.Helper()
	f := &fakeS3{bucket: bucket, objects: map[string]fakeObject{}, uploads: map[string]*fakeUpload{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func etagOf(b []byte) string {
	sum := md5.Sum(b) //nolint:gosec // see the import
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "/" + f.bucket + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	key := strings.TrimPrefix(r.URL.Path, prefix)
	q := r.URL.Query()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+key+"?"+r.URL.RawQuery)
	switch {
	case r.Method == http.MethodPut && q.Has("uploadId"):
		up, ok := f.uploads[q.Get("uploadId")]
		if !ok {
			writeS3Error(w, http.StatusNotFound, "NoSuchUpload")
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			writeS3Error(w, http.StatusBadRequest, "IncompleteBody")
			return
		}
		n, _ := strconv.Atoi(q.Get("partNumber"))
		up.parts[n] = b
		w.Header().Set("ETag", etagOf(b))
	case r.Method == http.MethodPut:
		b, err := io.ReadAll(r.Body)
		if err != nil {
			writeS3Error(w, http.StatusBadRequest, "IncompleteBody")
			return
		}
		f.objects[key] = fakeObject{data: b, contentType: r.Header.Get("Content-Type"), etag: etagOf(b), modTime: time.Now()}
		w.Header().Set("ETag", etagOf(b))
	case r.Method == http.MethodPost && q.Has("uploads"):
		f.nextID++
		id := strconv.Itoa(f.nextID)
		f.uploads[id] = &fakeUpload{key: key, contentType: r.Header.Get("Content-Type"), parts: map[int][]byte{}}
		writeXML(w, fmt.Sprintf("<InitiateMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>", f.bucket, key, id))
	case r.Method == http.MethodPost && q.Has("uploadId"):
		up, ok := f.uploads[q.Get("uploadId")]
		if !ok {
			writeS3Error(w, http.StatusNotFound, "NoSuchUpload")
			return
		}
		var body struct {
			Parts []struct {
				PartNumber int
				ETag       string
			} `xml:"Part"`
		}
		if err := xml.NewDecoder(r.Body).Decode(&body); err != nil {
			writeS3Error(w, http.StatusBadRequest, "MalformedXML")
			return
		}
		var data []byte
		for i, p := range body.Parts {
			b, ok := up.parts[p.PartNumber]
			if !ok || p.PartNumber != i+1 || p.ETag != etagOf(b) {
				writeS3Error(w, http.StatusBadRequest, "InvalidPart")
				return
			}
			data = append(data, b...)
		}
		delete(f.uploads, q.Get("uploadId"))
		f.objects[up.key] = fakeObject{data: data, contentType: up.contentType, etag: `"multipart"`, modTime: time.Now()}
		writeXML(w, fmt.Sprintf("<CompleteMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><ETag>&quot;multipart&quot;</ETag></CompleteMultipartUploadResult>", f.bucket, up.key))
	case r.Method == http.MethodDelete && q.Has("uploadId"):
		delete(f.uploads, q.Get("uploadId"))
		f.aborted++
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		o, ok := f.objects[key]
		if !ok {
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			writeS3Error(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		if m := r.Header.Get("If-Match"); m != "" && m != o.etag {
			writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed")
			return
		}
		data, status := o.data, http.StatusOK
		if rg := r.Header.Get("Range"); rg != "" {
			from, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			if err != nil || from >= len(o.data) {
				writeS3Error(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange")
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(o.data)-1, len(o.data)))
			data, status = o.data[from:], http.StatusPartialContent
		}
		w.Header().Set("Content-Type", o.contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("ETag", o.etag)
		w.Header().Set("Last-Modified", o.modTime.UTC().Format(http.TimeFormat))
		w.WriteHeader(status)
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	default:
		writeS3Error(w, http.StatusMethodNotAllowed, "MethodNotAllowed")
	}
}

// uploadCounts returns the number of the multipart uploads aborted, and of those neither
// completed nor aborted.
func (f *fakeS3) uploadCounts() (aborted, open int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.aborted, len(f.uploads)
}

// requestsMatching returns the requests whose "METHOD key?query" contains every one of parts.
func (f *fakeS3) requestsMatching(parts ...string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(f.requests), func(r string) bool {
		for _, p := range parts {
			if !strings.Contains(r, p) {
				return true
			}
		}
		return false
	})
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/xml")
	_, _ = io.WriteString(w, xml.Header+body) //nolint:gosec // a test server answering the store under test
}

func writeS3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, xml.Header+"<Error><Code>"+code+"</Code><Message>"+code+"</Message></Error>")
}
