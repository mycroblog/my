// SPDX-License-Identifier: AGPL-3.0-only

// Package shots is the evidence that a change of the web UI was looked at: the screenshots `scripts/task.sh
// web:shot` writes under .tmp/agentkit/shots with their manifest, and .tmp/agentkit/seen.json, where the
// PostToolUse hook records each screenshot the agent opened with the Read tool. `agentkit pr open` reads both.
package shots

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// Dir holds the screenshots and manifest.json, relative to the working tree.
	Dir = ".tmp/agentkit/shots"
	// SeenFile records the screenshots that were read, relative to the working tree.
	SeenFile = ".tmp/agentkit/seen.json"
)

// IsSource reports whether path is a source of the web UI whose change can alter a screen: a file of app/web/src
// or app/ui/src that is not a test, a story, a reference screenshot, or generated.
func IsSource(path string) bool {
	if !strings.HasPrefix(path, "app/web/src/") && !strings.HasPrefix(path, "app/ui/src/") {
		return false
	}
	base := filepath.Base(path)
	for _, part := range []string{".test.", ".stories.", ".spec.", ".gen."} {
		if strings.Contains(base, part) {
			return false
		}
	}
	for _, dir := range []string{"/__screenshots__/", "/__tests__/", "/__mocks__/"} {
		if strings.Contains(path, dir) {
			return false
		}
	}
	return true
}

// Sources returns the UI sources among paths, sorted.
func Sources(paths []string) []string {
	var out []string
	for _, p := range paths {
		if IsSource(p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Newest returns the latest modification time of sources in root and its path; deleted files do not count.
func Newest(root string, sources []string) (time.Time, string) {
	var newest time.Time
	var path string
	for _, p := range sources {
		if st, err := os.Stat(filepath.Join(root, p)); err == nil && st.ModTime().After(newest) {
			newest, path = st.ModTime(), p
		}
	}
	return newest, path
}

// Fingerprint identifies the content of sources in root, so that screenshots stay valid when a rebase rewrites
// the files without changing them.
func Fingerprint(root string, sources []string) string {
	h := sha256.New()
	for _, p := range sources {
		b, err := os.ReadFile(filepath.Join(root, p))
		h.Write([]byte(p))
		if err != nil {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
			h.Write(b)
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Shot is one screenshot of the manifest.
type Shot struct {
	Path  string    // relative to the working tree
	Story string    // the story it shows
	Taken time.Time // when it was taken
}

type manifest struct {
	Shots []struct {
		File    string          `json:"file"`
		Story   string          `json:"story"`
		TakenAt json.RawMessage `json:"takenAt"`
	} `json:"shots"`
}

// parseTime reads a manifest time: an RFC 3339 string, or a number of seconds or milliseconds since the epoch.
func parseTime(raw json.RawMessage) time.Time {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		t, _ := time.Parse(time.RFC3339Nano, s)
		return t
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		if n > 1e11 {
			return time.UnixMilli(int64(n))
		}
		return time.Unix(int64(n), 0)
	}
	return time.Time{}
}

// Load returns the screenshots of the manifest in root whose files exist. A screenshot without a readable
// takenAt has its file's modification time. No manifest is no screenshots.
func Load(root string) ([]Shot, error) {
	b, err := os.ReadFile(filepath.Join(root, Dir, "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	var out []Shot
	for _, s := range m.Shots {
		// The manifest may name the file alone, or by a path: the file is in Dir either way.
		rel := filepath.ToSlash(filepath.Join(Dir, filepath.Base(s.File)))
		st, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		taken := parseTime(s.TakenAt)
		if taken.IsZero() {
			taken = st.ModTime()
		}
		out = append(out, Shot{Path: rel, Story: s.Story, Taken: taken})
	}
	return out, nil
}

// Seen is one read of a screenshot.
type Seen struct {
	At time.Time `json:"at"`
	// Sources is the Fingerprint of the changed UI sources at the read, when the screenshot was newer than all of
	// them; empty otherwise.
	Sources string `json:"sources,omitempty"`
}

type seenFile struct {
	Seen map[string]Seen `json:"seen"`
}

// LoadSeen returns the recorded reads of root by screenshot path.
func LoadSeen(root string) map[string]Seen {
	var f seenFile
	if b, err := os.ReadFile(filepath.Join(root, SeenFile)); err == nil {
		_ = json.Unmarshal(b, &f)
	}
	if f.Seen == nil {
		f.Seen = map[string]Seen{}
	}
	return f.Seen
}

// Record stores that the screenshot rel of root was read at now. Hooks of parallel reads run at the same time,
// so the update holds a lock file.
func Record(root, rel string, now time.Time, sources string) error {
	file := filepath.Join(root, SeenFile)
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return err
	}
	lock := file + ".lock"
	for i := 0; ; i++ {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			break
		}
		// A lock older than a few seconds is a hook that died.
		if st, serr := os.Stat(lock); i >= 100 || (serr == nil && time.Since(st.ModTime()) > 5*time.Second) {
			_ = os.Remove(lock)
			continue
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer func() { _ = os.Remove(lock) }()
	seen := LoadSeen(root)
	seen[rel] = Seen{At: now, Sources: sources}
	b, err := json.MarshalIndent(seenFile{seen}, "", "  ")
	if err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

// Verdict is what the screenshots of root say about the change of sources.
type Verdict struct {
	Looked []string // screenshots of the present sources that were read after they were taken
	Unseen []string // screenshots of the present sources not read since
	Newest string   // the most recently modified source
}

// Judge sorts the screenshots of root for the changed UI sources. A screenshot shows the present sources when it
// was taken after the newest of them changed, or when it was read while that held and the sources have the same
// content now.
func Judge(root string, sources []string) (Verdict, error) {
	all, err := Load(root)
	if err != nil {
		return Verdict{}, err
	}
	newest, path := Newest(root, sources)
	fp := Fingerprint(root, sources)
	seen := LoadSeen(root)
	v := Verdict{Newest: path}
	for _, s := range all {
		se := seen[s.Path]
		read := !se.At.IsZero() && !se.At.Before(s.Taken.Truncate(time.Second))
		switch {
		case read && se.Sources != "" && se.Sources == fp:
			v.Looked = append(v.Looked, s.Path)
		case s.Taken.Before(newest.Truncate(time.Second)):
			// taken before the last edit
		case read:
			v.Looked = append(v.Looked, s.Path)
		default:
			v.Unseen = append(v.Unseen, s.Path)
		}
	}
	return v, nil
}

// Fresh reports whether the screenshot rel of root was taken after the newest of sources changed.
func Fresh(root, rel string, sources []string) bool {
	all, err := Load(root)
	if err != nil {
		return false
	}
	newest, _ := Newest(root, sources)
	for _, s := range all {
		if s.Path == rel {
			return !s.Taken.Before(newest.Truncate(time.Second))
		}
	}
	return false
}
