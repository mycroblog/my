// SPDX-License-Identifier: AGPL-3.0-only

package ops

import "testing"

func TestCollectQueuedReadsEveryPage(t *testing.T) {
	entry := &struct {
		State string `json:"state"`
	}{State: "QUEUED"}
	node := func(n int, queued bool) struct {
		Number          int `json:"number"`
		MergeQueueEntry *struct {
			State string `json:"state"`
		} `json:"mergeQueueEntry"`
	} {
		v := struct {
			Number          int `json:"number"`
			MergeQueueEntry *struct {
				State string `json:"state"`
			} `json:"mergeQueueEntry"`
		}{Number: n}
		if queued {
			v.MergeQueueEntry = entry
		}
		return v
	}
	first := queuePage{Nodes: append([]struct {
		Number          int `json:"number"`
		MergeQueueEntry *struct {
			State string `json:"state"`
		} `json:"mergeQueueEntry"`
	}{}, node(1, true), node(2, false))}
	first.PageInfo.HasNextPage, first.PageInfo.EndCursor = true, "c1"
	second := queuePage{Nodes: append([]struct {
		Number          int `json:"number"`
		MergeQueueEntry *struct {
			State string `json:"state"`
		} `json:"mergeQueueEntry"`
	}{}, node(3, true))}
	calls := 0
	got, err := collectQueued(func(cursor string) (queuePage, error) {
		calls++
		if cursor == "" {
			return first, nil
		}
		if cursor != "c1" {
			t.Errorf("unexpected cursor %q", cursor)
		}
		return second, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !got[1] || got[2] || !got[3] {
		t.Errorf("calls %d, got %v: the entry on the second page must be found", calls, got)
	}
}
