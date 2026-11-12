// SPDX-License-Identifier: AGPL-3.0-only

// Package feedback reads and answers the batches of notes that the overlay of @my/devtools sends from the dev server
// (ADR-0168): one JSON file per batch in .tmp/feedback, whose shape web/devtools/src/note.ts defines.
package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Dir is the directory of the batches, and of the ServerFile, of the working tree at root.
func Dir(root string) string { return filepath.Join(root, ".tmp", "feedback") }

// Rect is a box in page coordinates.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Reply is the agent's answer to a note.
type Reply struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	// Options is, for the kind options, the id of the <Options> the agent placed on the page.
	Options string `json:"options,omitempty"`
	At      string `json:"at"`
}

// Pick is the variant the decider picked of an <Options>.
type Pick struct {
	Options  string   `json:"options"`
	Variant  string   `json:"variant"`
	Variants []string `json:"variants"`
}

// Note is one note of a batch.
type Note struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Target struct {
		Tag      string `json:"tag"`
		Selector string `json:"selector"`
		Text     string `json:"text"`
		Rect     Rect   `json:"rect"`
		Area     *Rect  `json:"area"`
	} `json:"target"`
	Source  *string  `json:"source"`
	Sources []string `json:"sources"`
	Layer   *string  `json:"layer"`
	State   struct {
		Page     string  `json:"page"`
		Route    *string `json:"route"`
		Viewport struct {
			Width            float64 `json:"width"`
			Height           float64 `json:"height"`
			DevicePixelRatio float64 `json:"devicePixelRatio"`
		} `json:"viewport"`
		Theme struct {
			Page   *string `json:"page"`
			System string  `json:"system"`
		} `json:"theme"`
		Locale      string  `json:"locale"`
		Direction   string  `json:"direction"`
		Orientation *string `json:"orientation"`
		Account     *string `json:"account"`
	} `json:"state"`
	Reproduction struct {
		Scenario *string `json:"scenario"`
		Step     *string `json:"step"`
	} `json:"reproduction"`
	Accessibility string `json:"accessibility"`
	Pick          *Pick  `json:"pick,omitempty"`
	Reply         *Reply `json:"reply,omitempty"`
}

// Batch is what the decider sent at once.
type Batch struct {
	ID        string `json:"id"`
	SentAt    string `json:"sentAt"`
	Page      string `json:"page"`
	Notes     []Note `json:"notes"`
	Delivered string `json:"delivered,omitempty"`
}

// IDs returns the ids of the batches in dir, oldest first.
func IDs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if name := e.Name(); !e.IsDir() && strings.HasSuffix(name, ".json") && name != ServerFile {
			ids = append(ids, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// Load reads one batch.
func Load(dir, id string) (*Batch, error) {
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var b Batch
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("batch %s: %w", id, err)
	}
	return &b, nil
}

// Latest reads the newest batch.
func Latest(dir string) (*Batch, error) {
	ids, err := IDs(dir)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no batch in %s: the decider sends one from the overlay of the dev server", dir)
	}
	return Load(dir, ids[len(ids)-1])
}

// Next reads the oldest batch that no wait has delivered, or returns nil.
func Next(dir string) (*Batch, error) {
	ids, err := IDs(dir)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		b, err := Load(dir, id)
		if err != nil {
			// A file the dev server is still writing; the next poll reads it whole.
			continue
		}
		if b.Delivered == "" {
			return b, nil
		}
	}
	return nil, nil
}

// update rewrites one batch file through f, keeping the fields this package does not know.
func update(dir, id string, f func(batch map[string]any) error) error {
	path := filepath.Join(dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var batch map[string]any
	if err := json.Unmarshal(data, &batch); err != nil {
		return fmt.Errorf("batch %s: %w", id, err)
	}
	if err := f(batch); err != nil {
		return err
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(batch); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Wait returns the oldest undelivered batch once there is one, marked delivered, polling every interval.
func Wait(ctx context.Context, dir string, interval time.Duration, now func() time.Time) (*Batch, error) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		b, err := Next(dir)
		if err != nil {
			return nil, err
		}
		if b != nil {
			b.Delivered = now().UTC().Format(time.RFC3339)
			return b, update(dir, b.ID, func(batch map[string]any) error {
				batch["delivered"] = b.Delivered
				return nil
			})
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

// Kinds are the kinds of a reply: fixed, a question, or options the decider switches between on the page.
var Kinds = []string{"fixed", "question", "options"}

// Answer writes the reply to a note: "<batch>.<n>", or "<n>" for a note of the newest batch; options is the id of
// the <Options> of a reply of the kind options. It returns the note's id.
func Answer(dir, note, kind, options, text string, now time.Time) (string, error) {
	if (kind == "options") != (options != "") {
		return "", fmt.Errorf("a reply of the kind options, and only one, names the id of its <Options> (--options)")
	}
	batch, n, ok := strings.Cut(note, ".")
	if !ok {
		latest, err := Latest(dir)
		if err != nil {
			return "", err
		}
		batch, n = latest.ID, note
	}
	i, err := strconv.Atoi(n)
	if err != nil || i < 1 {
		return "", fmt.Errorf("note %q: expected <batch>.<n> or <n> (feedback show lists them)", note)
	}
	id := batch + "." + n
	return id, update(dir, batch, func(b map[string]any) error {
		notes, _ := b["notes"].([]any)
		if i > len(notes) {
			return fmt.Errorf("batch %s has %d notes, not %d", batch, len(notes), i)
		}
		m, _ := notes[i-1].(map[string]any)
		if m == nil {
			return fmt.Errorf("note %s is not an object", id)
		}
		reply := map[string]any{"kind": kind, "text": text, "at": now.UTC().Format(time.RFC3339)}
		if options != "" {
			reply["options"] = options
		}
		m["reply"] = reply
		return nil
	})
}

func or(s *string, none string) string {
	if s == nil || *s == "" {
		return none
	}
	return *s
}

// Print writes a batch for an agent to read, each note with its source, layer, and state.
func Print(w io.Writer, b *Batch) {
	_, _ = fmt.Fprintf(w, "batch %s, sent %s from %s, %d notes\n", b.ID, b.SentAt, b.Page, len(b.Notes))
	_, _ = fmt.Fprintln(w, "answer each: scripts/agentkit.sh feedback reply <note> [--kind fixed|question] <text>")
	for i := range b.Notes {
		_, _ = fmt.Fprintln(w)
		PrintNote(w, &b.Notes[i])
	}
}

// PrintNote writes one note.
func PrintNote(w io.Writer, n *Note) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format+"\n", a...) }
	p("%s  %q", n.ID, n.Text)
	if n.Pick != nil {
		p("  pick: %s of the options %s (%s): keep it, remove the <Options> and the other variants",
			n.Pick.Variant, n.Pick.Options, strings.Join(n.Pick.Variants, ", "))
	}
	p("  source: %s (layer %s)", or(n.Source, "none: no data-source on the element or its ancestors"), or(n.Layer, "unknown"))
	t := n.Target
	p("  element: <%s> %q, selector %q, at %g,%g %gx%g", t.Tag, t.Text, t.Selector, t.Rect.X, t.Rect.Y, t.Rect.Width, t.Rect.Height)
	if t.Area != nil {
		p("  area: at %g,%g %gx%g, sources %s", t.Area.X, t.Area.Y, t.Area.Width, t.Area.Height, strings.Join(n.Sources, ", "))
	}
	s := n.State
	p("  page: %s, route %s", s.Page, or(s.Route, "none (the page has no router)"))
	p("  state: viewport %gx%g@%g, theme %s (system %s), locale %s, direction %s, orientation %s, account %s",
		s.Viewport.Width, s.Viewport.Height, s.Viewport.DevicePixelRatio, or(s.Theme.Page, "unset"), s.Theme.System,
		s.Locale, s.Direction, or(s.Orientation, "unknown"), or(s.Account, "none"))
	if r := n.Reproduction; r.Scenario != nil {
		p("  reproduce: scenario %s, step %s", *r.Scenario, or(r.Step, "-"))
	} else {
		p("  reproduce: no E2E scenario opens the page; open %s at the viewport above", s.Page)
	}
	if n.Accessibility != "" {
		p("  accessibility:")
		for _, line := range strings.Split(n.Accessibility, "\n") {
			p("    %s", line)
		}
	}
	if n.Reply != nil {
		kind := n.Reply.Kind
		if n.Reply.Options != "" {
			kind += " " + n.Reply.Options
		}
		p("  reply (%s, %s): %s", kind, n.Reply.At, n.Reply.Text)
	}
}
