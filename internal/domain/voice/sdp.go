// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"errors"
	"strings"
)

// The errors of CheckSDP.
var (
	// ErrSDPMalformed reports a session description that does not start with its version line.
	ErrSDPMalformed = errors.New("voice: the session description is malformed")
	// ErrSDPMedia reports a media section other than audio: the rooms carry audio only, without
	// video or data channels (ADR-0101 §4).
	ErrSDPMedia = errors.New("voice: the session description has a media section other than audio")
	// ErrSDPDirection reports an audio section that sends from a participant who may not speak.
	ErrSDPDirection = errors.New("voice: the session description sends audio from a participant who may not speak")
)

// The directions of a media section (RFC 8866 §6.7).
const (
	sdpSendRecv = "sendrecv"
	sdpSendOnly = "sendonly"
	sdpRecvOnly = "recvonly"
	sdpInactive = "inactive"
)

// CheckSDP checks the session description of an offer or an answer that the relay passes on in
// a room (ADR-0101 §4): every media section (m= line) is audio, and when the sender may not
// publish audio (it is not a speaker and may not speak freely), every audio section is recvonly
// or inactive. The direction of a section is its own direction attribute, else the one of the
// session level, else sendrecv (RFC 8866 §6.7).
func CheckSDP(sdp string, mayPublish bool) error {
	lines := strings.Split(strings.ReplaceAll(sdp, "\r\n", "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "v=") {
		return ErrSDPMalformed
	}
	sessionDir := ""
	var sections []string // the direction of each media section, "" without its own
	for _, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "m="):
			media, _, _ := strings.Cut(strings.TrimPrefix(line, "m="), " ")
			if media != "audio" {
				return ErrSDPMedia
			}
			sections = append(sections, "")
		case strings.HasPrefix(line, "a="):
			dir := strings.TrimSpace(strings.TrimPrefix(line, "a="))
			switch dir {
			case sdpSendRecv, sdpSendOnly, sdpRecvOnly, sdpInactive:
			default:
				continue
			}
			if len(sections) == 0 {
				sessionDir = dir
			} else {
				sections[len(sections)-1] = dir
			}
		}
	}
	if mayPublish {
		return nil
	}
	for _, dir := range sections {
		if dir == "" {
			dir = sessionDir
		}
		if dir != sdpRecvOnly && dir != sdpInactive {
			return ErrSDPDirection
		}
	}
	return nil
}
