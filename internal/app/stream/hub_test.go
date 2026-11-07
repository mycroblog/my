// SPDX-License-Identifier: AGPL-3.0-only

package stream_test

import (
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/app/stream"
)

// The routing, limits, and slow subscribers of Hub are verified by streamtest.StreamBrokerContract
// (pkg/testkit/streamtest/stream_test.go).

func isDone(s stream.Subscription) bool {
	select {
	case <-s.Done():
		return true
	default:
		return false
	}
}

func TestHubClose(t *testing.T) {
	h := stream.NewHub(stream.HubOptions{})
	s, err := h.Subscribe(t.Context(), aliceID)
	if err != nil {
		t.Fatal(err)
	}
	h.Close()
	if !isDone(s) {
		t.Error("Close left a subscription open")
	}
	if _, err := h.Subscribe(t.Context(), aliceID); !errors.Is(err, stream.ErrClosed) {
		t.Errorf("Subscribe after Close: %v, want ErrClosed", err)
	}
	h.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: entryID})
	if got := h.Len(); got != 0 {
		t.Errorf("Len = %d, want 0", got)
	}
}
