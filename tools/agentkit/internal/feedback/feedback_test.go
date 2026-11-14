// SPDX-License-Identifier: AGPL-3.0-only

package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// batchJSON is a batch as the dev server writes it, with a field this package does not know.
const batchJSON = `{
  "id": "%s",
  "sentAt": "2026-11-12T06:30:45.000Z",
  "page": "/",
  "notes": [
    {
      "id": "%s.1",
      "text": "too tight",
      "target": {"tag": "p", "selector": "main > p", "text": "The quick", "rect": {"x": 24, "y": 120, "width": 900, "height": 40}, "area": null},
      "source": "web/catalog/src/Catalog.tsx:22",
      "sources": [],
      "layer": "@my/catalog",
      "state": {"page": "/", "route": null, "viewport": {"width": 1280, "height": 720, "devicePixelRatio": 1},
        "theme": {"page": null, "system": "light"}, "locale": "en", "direction": "ltr", "orientation": null, "account": null},
      "reproduction": {"scenario": null, "step": null},
      "accessibility": "paragraph\n  text \"The quick\"",
      "extra": {"kept": true}
    }
  ]
}`

func write(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeBatch(dir, id); err != nil {
		t.Fatal(err)
	}
}

func writeBatch(dir, id string) error {
	return os.WriteFile(filepath.Join(dir, id+".json"), []byte(strings.ReplaceAll(batchJSON, "%s", id)), 0o600)
}

var clock = func() time.Time { return time.Date(2026, 11, 12, 7, 0, 0, 0, time.UTC) }

func TestWaitDeliversEachBatchOnce(t *testing.T) {
	dir := Dir(t.TempDir())
	write(t, dir, "20261112-153045-0001")
	b, err := Wait(context.Background(), dir, time.Millisecond, clock)
	if err != nil || b.ID != "20261112-153045-0001" || b.Delivered != "2026-11-12T07:00:00Z" {
		t.Fatalf("Wait = %+v, %v", b, err)
	}
	if next, err := Next(dir); next != nil || err != nil {
		t.Fatalf("Next after delivery = %+v, %v; want none", next, err)
	}

	// A batch written while Wait looks, 20ms later on the bubble's clock.
	synctest.Test(t, func(t *testing.T) {
		go func() {
			<-time.After(20 * time.Millisecond)
			_ = writeBatch(dir, "20261112-153100-0002")
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		b, err := Wait(ctx, dir, 5*time.Millisecond, clock)
		if err != nil || b.ID != "20261112-153100-0002" {
			t.Fatalf("Wait for a later batch = %+v, %v", b, err)
		}
	})
}

func TestWaitTimesOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := Wait(ctx, Dir(t.TempDir()), time.Millisecond, clock); err == nil {
		t.Fatal("Wait on an empty directory returned")
	}
}

func TestAnswerKeepsUnknownFields(t *testing.T) {
	dir := Dir(t.TempDir())
	write(t, dir, "20261112-153045-0001")
	id, err := Answer(dir, "1", "question", "", "which weight?", clock())
	if err != nil || id != "20261112-153045-0001.1" {
		t.Fatalf("Answer = %q, %v", id, err)
	}
	b, err := Load(dir, "20261112-153045-0001")
	if err != nil || b.Notes[0].Reply == nil || *b.Notes[0].Reply != (Reply{Kind: "question", Text: "which weight?", At: "2026-11-12T07:00:00Z"}) {
		t.Fatalf("reply = %+v, %v", b.Notes[0].Reply, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "20261112-153045-0001.json"))
	var raw struct {
		Notes []struct {
			Extra struct {
				Kept bool `json:"kept"`
			} `json:"extra"`
		} `json:"notes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || !raw.Notes[0].Extra.Kept {
		t.Fatalf("the unknown field was lost: %s", data)
	}
	if _, err := Answer(dir, "20261112-153045-0001.2", "fixed", "", "x", clock()); err == nil {
		t.Fatal("Answer to a note the batch does not have succeeded")
	}
}

func TestAnswerWithOptions(t *testing.T) {
	dir := Dir(t.TempDir())
	write(t, dir, "20261112-153045-0001")
	if _, err := Answer(dir, "1", "options", "", "two shapes", clock()); err == nil {
		t.Fatal("options without an id succeeded")
	}
	if _, err := Answer(dir, "1", "fixed", "swatch", "done", clock()); err == nil {
		t.Fatal("an id on a fixed reply succeeded")
	}
	if _, err := Answer(dir, "1", "options", "swatch", "two shapes", clock()); err != nil {
		t.Fatal(err)
	}
	b, _ := Load(dir, "20261112-153045-0001")
	var out bytes.Buffer
	PrintNote(&out, &b.Notes[0])
	if !strings.Contains(out.String(), "reply (options swatch, 2026-11-12T07:00:00Z): two shapes") {
		t.Fatalf("PrintNote:\n%s", out.String())
	}
}

func TestPrintPick(t *testing.T) {
	n := Note{ID: "b.1", Text: "Pick round of swatch.", Pick: &Pick{Options: "swatch", Variant: "round", Variants: []string{"square", "round"}}}
	var out bytes.Buffer
	PrintNote(&out, &n)
	if !strings.Contains(out.String(), "pick: round of the options swatch (square, round)") ||
		!strings.Contains(out.String(), "\n  reason: none given\n") {
		t.Fatalf("PrintNote:\n%s", out.String())
	}
	n.Pick.Reason = "softer next to the avatars"
	out.Reset()
	PrintNote(&out, &n)
	if !strings.Contains(out.String(), "and the other variants\n  reason: softer next to the avatars\n") {
		t.Fatalf("PrintNote lacks the reason next to the pick:\n%s", out.String())
	}
}

func TestPrint(t *testing.T) {
	dir := Dir(t.TempDir())
	write(t, dir, "20261112-153045-0001")
	b, err := Latest(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	Print(&out, b)
	for _, want := range []string{
		`20261112-153045-0001.1  "too tight"`,
		"source: web/catalog/src/Catalog.tsx:22 (layer @my/catalog)",
		"route none (the page has no router)",
		"no E2E scenario opens the page",
		`      text "The quick"`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Print lacks %q:\n%s", want, out.String())
		}
	}
}
