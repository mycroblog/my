// SPDX-License-Identifier: AGPL-3.0-only

package voice

import "testing"

func TestCheckCandidate(t *testing.T) {
	c := `candidate:842163049 1 udp 1677729535 192.0.2.1 3478 typ srflx raddr 0.0.0.0 rport 0 generation 0`
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"full", `{"candidate":"` + c + `","sdpMid":"0","sdpMLineIndex":0,"usernameFragment":"abcd"}`, true},
		{"candidate only", `{"candidate":"` + c + `"}`, true},
		{"end of the candidates", `{"candidate":"","sdpMid":"0","sdpMLineIndex":0}`, true},
		{"null sdpMid", `{"candidate":"` + c + `","sdpMid":null}`, true},
		{"no candidate", `{"sdpMid":"0"}`, false},
		{"another key", `{"candidate":"` + c + `","extra":1}`, false},
		{"not a candidate attribute", `{"candidate":"a=sendrecv"}`, false},
		{"a line break", `{"candidate":"` + c + `\r\na=sendrecv"}`, false},
		{"a line break in sdpMid", `{"candidate":"` + c + `","sdpMid":"0\n"}`, false},
		{"a negative index", `{"candidate":"` + c + `","sdpMLineIndex":-1}`, false},
		{"a string index", `{"candidate":"` + c + `","sdpMLineIndex":"0"}`, false},
		{"not JSON", c, false},
		{"an array", `["candidate:1"]`, false},
		{"null", `null`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckCandidate(tt.in); (err == nil) != tt.ok {
				t.Errorf("CheckCandidate = %v, want ok %v", err, tt.ok)
			}
		})
	}
}
