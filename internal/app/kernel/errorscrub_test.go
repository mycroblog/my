// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

func TestScrubMessage(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, in, want string }{
		{"url", `Get "https://remote.example/users/alice?x=1": EOF`, `Get "<url>": EOF`},
		{"email", "send to alice@example.com failed", "send to <email> failed"},
		{"acct", "no actor @alice@remote.example", "no actor <acct>"},
		{"ipv4", "dial tcp 10.0.0.12:5432: connect: connection refused", "dial tcp <ip>:5432: connect: connection refused"},
		{"ipv6", "dial tcp [2001:db8::1]:443: i/o timeout", "dial tcp [<ip>]:443: i/o timeout"},
		{"ipv6 loopback", "listen on ::1 failed", "listen on <ip> failed"},
		{"time is no ip", "at 12:30:45 the job stopped", "at 12:30:45 the job stopped"},
		{"uuid", "status 0192d5a8-7c1e-7b3a-9f00-123456789abc not found", "status <id> not found"},
		{"ulid", "job 01ARZ3NDEKTSV4RRFFQ69G5FAV failed", "job <id> failed"},
		{"long number", "account 12345678901 and port 5432", "account <n> and port 5432"},
		{"nothing", "sql: no rows in result set", "sql: no rows in result set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := kernel.ScrubErrorMessage(tt.in); got != tt.want {
				t.Errorf("kernel.ScrubErrorMessage(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestScrubMessageCut(t *testing.T) {
	t.Parallel()
	got := kernel.ScrubErrorMessage(strings.Repeat("é", 1000)) // 2,000 bytes
	if len(got) > kernel.MaxErrorMessageBytes || len(got) < kernel.MaxErrorMessageBytes-1 || !strings.HasPrefix(got, "é") || strings.ContainsRune(got, '\uFFFD') {
		t.Errorf("cut to %d bytes, want 1 KiB of whole characters", len(got))
	}
}
