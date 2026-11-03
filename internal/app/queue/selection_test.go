// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/queue"
)

func TestSelectJobTypes(t *testing.T) {
	registered := []string{"media.process", "federation.inbox", "federation.deliver", "notification.push"}
	tests := []struct {
		name            string
		include, except []string
		run, skip       []string
	}{
		{
			name:    "an exact name",
			include: []string{"media.process"},
			run:     []string{"media.process"},
			skip:    []string{"federation.deliver", "federation.inbox", "notification.push"},
		},
		{
			name:    "a prefix and a name",
			include: []string{"federation.*", "media.process"},
			run:     []string{"federation.deliver", "federation.inbox", "media.process"},
			skip:    []string{"notification.push"},
		},
		{
			name:   "an exclude list turns into the types left",
			except: []string{"federation.*"},
			run:    []string{"media.process", "notification.push"},
			skip:   []string{"federation.deliver", "federation.inbox"},
		},
		{
			name:    "overlapping patterns",
			include: []string{"federation.*", "federation.deliver"},
			run:     []string{"federation.deliver", "federation.inbox"},
			skip:    []string{"media.process", "notification.push"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, skip, err := queue.SelectJobTypes(registered, tt.include, tt.except)
			if err != nil {
				t.Fatalf("SelectJobTypes: %v", err)
			}
			if !slices.Equal(run, tt.run) || !slices.Equal(skip, tt.skip) {
				t.Errorf("run, skip = %v, %v; want %v, %v", run, skip, tt.run, tt.skip)
			}
		})
	}
}

func TestSelectJobTypesRefuses(t *testing.T) {
	registered := []string{"federation.deliver", "federation.inbox"}
	for _, tt := range []struct {
		name            string
		include, except []string
		want            string
	}{
		{name: "an unknown name", include: []string{"federation.delivr"}, want: "federation.delivr"},
		{name: "an unknown prefix", except: []string{"voice.*"}, want: "voice.*"},
		{name: "a prefix without the dot does not stand for a name", include: []string{"federation"}, want: "federation"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := queue.SelectJobTypes(registered, tt.include, tt.except)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want one that names %q", err, tt.want)
			}
		})
	}
	if _, _, err := queue.SelectJobTypes(registered, nil, []string{"federation.*"}); !errors.Is(err, queue.ErrNoJobTypes) {
		t.Errorf("exclude everything: err = %v, want ErrNoJobTypes", err)
	}
}
