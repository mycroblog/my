// SPDX-License-Identifier: AGPL-3.0-only

package gcs_test

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGCS is an in-memory server of the parts of the JSON API of Cloud Storage that the Store
// uses: the multipart and resumable uploads, the object's metadata and media (with a Range from
// a position and a generation), and the deletes. It also answers the IAM signBlob method, with
// key, and the service account's email of the metadata server, so that a client whose requests
// all reach it runs without Google Cloud.
type fakeGCS struct {
	bucket string
	key    *rsa.PrivateKey

	mu       sync.Mutex
	objects  map[string]fakeObject
	uploads  map[string]*fakeUpload
	nextGen  int64
	nextID   int
	requests []string // "METHOD host path?query" of every request
}

type fakeObject struct {
	data        []byte
	contentType string
	generation  int64
	updated     time.Time
}

type fakeUpload struct {
	name        string
	contentType string
	data        []byte
}

// serviceAccount is the email of the service account of the metadata server and of the signBlob
// requests.
const serviceAccount = "my-server@project.iam.gserviceaccount.com"

func newFakeGCS(t *testing.T, bucket string, key *rsa.PrivateKey) (*fakeGCS, *httptest.Server) {
	t.Helper()
	f := &fakeGCS{bucket: bucket, key: key, objects: map[string]fakeObject{}, uploads: map[string]*fakeUpload{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

// redirect sends every request to srv, whatever its host, keeping the original host in the
// X-Original-Host header.
type redirect struct{ srv *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Original-Host", req.URL.Host)
	req.URL.Scheme, req.URL.Host = r.srv.Scheme, r.srv.Host
	req.Host = r.srv.Host
	return http.DefaultTransport.RoundTrip(req)
}

func (f *fakeGCS) requestsMatching(parts ...string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.requests {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(r, p)
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeGCS) openUploads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.uploads)
}

func (f *fakeGCS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The body is read whole before anything is stored, so that an upload that the client
	// abandons writes nothing and does not hold the lock.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	f.mu.Lock()
	defer f.mu.Unlock()
	host := r.Header.Get("X-Original-Host")
	f.requests = append(f.requests, r.Method+" "+host+" "+r.URL.Path+"?"+r.URL.RawQuery)
	objPrefix := "/storage/v1/b/" + f.bucket + "/o/"
	uploadPath := "/upload/storage/v1/b/" + f.bucket + "/o"
	switch {
	case r.URL.Path == "/computeMetadata/v1/instance/service-accounts/default/email":
		w.Header().Set("Metadata-Flavor", "Google")
		_, _ = io.WriteString(w, serviceAccount)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":signBlob"):
		f.signBlob(w, r)
	case r.URL.Path == uploadPath && r.URL.Query().Has("upload_id"):
		f.resumeUpload(w, r)
	case r.URL.Path == uploadPath && r.Method == http.MethodPost && r.URL.Query().Get("uploadType") == "multipart":
		f.multipartUpload(w, r)
	case r.URL.Path == uploadPath && r.Method == http.MethodPost && r.URL.Query().Get("uploadType") == "resumable":
		f.startResumable(w, r)
	case strings.HasPrefix(r.URL.Path, objPrefix) && r.Method == http.MethodGet:
		f.get(w, r, strings.TrimPrefix(r.URL.Path, objPrefix))
	case strings.HasPrefix(r.URL.Path, objPrefix) && r.Method == http.MethodDelete:
		name := strings.TrimPrefix(r.URL.Path, objPrefix)
		if _, ok := f.objects[name]; !ok {
			writeError(w, http.StatusNotFound)
			return
		}
		delete(f.objects, name)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusNotFound)
	}
}

func writeError(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":%q}}`, code, http.StatusText(code))
}

func (f *fakeGCS) signBlob(w http.ResponseWriter, r *http.Request) {
	var req struct{ Payload string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	payload, err := base64.StdEncoding.DecodeString(req.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(payload)
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, sum[:])
	if err != nil {
		writeError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"keyId": "key-1", "signedBlob": base64.StdEncoding.EncodeToString(sig)})
}

// objectMeta is the metadata of an upload.
type objectMeta struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
}

func (f *fakeGCS) multipartUpload(w http.ResponseWriter, r *http.Request) {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	mr := multipart.NewReader(r.Body, params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	var meta objectMeta
	if err := json.NewDecoder(part).Decode(&meta); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	part, err = mr.NextPart()
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(part)
	if err != nil {
		// The client went away mid-upload: nothing is written.
		writeError(w, http.StatusBadRequest)
		return
	}
	if meta.ContentType == "" {
		meta.ContentType = part.Header.Get("Content-Type")
	}
	f.store(w, meta.Name, meta.ContentType, data)
}

func (f *fakeGCS) store(w http.ResponseWriter, name, contentType string, data []byte) {
	f.nextGen++
	obj := fakeObject{data: data, contentType: contentType, generation: f.nextGen, updated: time.Now().UTC()}
	f.objects[name] = obj
	f.writeMeta(w, name, obj)
}

func (f *fakeGCS) writeMeta(w http.ResponseWriter, name string, obj fakeObject) {
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(obj.data, crc32.MakeTable(crc32.Castagnoli)))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"bucket": f.bucket, "name": name, "contentType": obj.contentType,
		"size": strconv.Itoa(len(obj.data)), "generation": strconv.FormatInt(obj.generation, 10), "metageneration": "1",
		"updated": obj.updated.Format(time.RFC3339Nano), "crc32c": base64.StdEncoding.EncodeToString(crc[:]),
	})
}

func (f *fakeGCS) startResumable(w http.ResponseWriter, r *http.Request) {
	var meta objectMeta
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	if meta.Name == "" {
		meta.Name = r.URL.Query().Get("name")
	}
	f.nextID++
	id := strconv.Itoa(f.nextID)
	f.uploads[id] = &fakeUpload{name: meta.Name, contentType: meta.ContentType}
	loc := "http://" + r.Host + "/upload/storage/v1/b/" + f.bucket + "/o?uploadType=resumable&upload_id=" + id
	w.Header().Set("Location", loc)
	w.WriteHeader(http.StatusOK)
}

// resumeUpload takes a chunk of a resumable upload (a POST or a PUT): Content-Range "bytes a-b/*" for a chunk that
// is not the last, "bytes a-b/N" or "bytes */N" for the last.
func (f *fakeGCS) resumeUpload(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("upload_id")
	up, ok := f.uploads[id]
	if !ok {
		writeError(w, http.StatusNotFound)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest)
		return
	}
	cr := strings.TrimPrefix(r.Header.Get("Content-Range"), "bytes ")
	rng, total, _ := strings.Cut(cr, "/")
	if rng != "*" {
		start, _, _ := strings.Cut(rng, "-")
		if s, err := strconv.Atoi(start); err != nil || s != len(up.data) {
			writeError(w, http.StatusBadRequest)
			return
		}
		up.data = append(up.data, data...)
	}
	if total == "*" {
		w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", len(up.data)-1))
		// The client asks for a 200 with the status in a header instead of a 308.
		if r.Header.Get("X-GUploader-No-308") == "yes" {
			w.Header().Set("X-HTTP-Status-Code-Override", "308")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusPermanentRedirect)
		return
	}
	if n, err := strconv.Atoi(total); err != nil || n != len(up.data) {
		writeError(w, http.StatusBadRequest)
		return
	}
	delete(f.uploads, id)
	f.store(w, up.name, up.contentType, up.data)
}

func (f *fakeGCS) get(w http.ResponseWriter, r *http.Request, name string) {
	obj, ok := f.objects[name]
	if !ok {
		writeError(w, http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	if g := q.Get("generation"); g != "" && g != strconv.FormatInt(obj.generation, 10) {
		writeError(w, http.StatusNotFound)
		return
	}
	if q.Get("alt") != "media" {
		f.writeMeta(w, name, obj)
		return
	}
	h := w.Header()
	h.Set("Content-Type", obj.contentType)
	h.Set("Last-Modified", obj.updated.Format(http.TimeFormat))
	h.Set("X-Goog-Generation", strconv.FormatInt(obj.generation, 10))
	h.Set("X-Goog-Metageneration", "1")
	data := obj.data
	if rng := r.Header.Get("Range"); rng != "" {
		start, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rng, "bytes="), "-"))
		if err != nil || start >= len(data) {
			writeError(w, http.StatusRequestedRangeNotSatisfiable)
			return
		}
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
		h.Set("Content-Length", strconv.Itoa(len(data)-start))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.Copy(w, bytes.NewReader(data[start:]))
		return
	}
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli)))
	h.Set("X-Goog-Hash", "crc32c="+base64.StdEncoding.EncodeToString(crc[:]))
	h.Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}
