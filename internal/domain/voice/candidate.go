// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrCandidate reports an ICE candidate that is not the JSON of an RTCIceCandidateInit.
var ErrCandidate = errors.New("voice: the ICE candidate is malformed")

// CheckCandidate checks the ICE candidate of a signal that the relay passes on (ADR-0101 §4):
// the JSON of an RTCIceCandidateInit, an object of candidate, sdpMid, sdpMLineIndex, and
// usernameFragment only, whose candidate is empty (the end of the candidates) or a candidate
// attribute ("candidate:…", RFC 8839 §5.1) on one line.
func CheckCandidate(s string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &fields); err != nil || fields == nil {
		return ErrCandidate
	}
	var c struct {
		Candidate        *string `json:"candidate"`
		SDPMid           *string `json:"sdpMid"`
		SDPMLineIndex    *uint16 `json:"sdpMLineIndex"`
		UsernameFragment *string `json:"usernameFragment"`
	}
	for k := range fields {
		switch k {
		case "candidate", "sdpMid", "sdpMLineIndex", "usernameFragment":
		default:
			return ErrCandidate
		}
	}
	if err := json.Unmarshal([]byte(s), &c); err != nil || c.Candidate == nil {
		return ErrCandidate
	}
	for _, v := range []*string{c.Candidate, c.SDPMid, c.UsernameFragment} {
		if v != nil && strings.ContainsAny(*v, "\r\n") {
			return ErrCandidate
		}
	}
	if *c.Candidate != "" && !strings.HasPrefix(*c.Candidate, "candidate:") {
		return ErrCandidate
	}
	return nil
}
