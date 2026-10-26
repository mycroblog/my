// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"errors"
	"strings"
	"testing"
)

// sdp joins the lines of a session description with CRLF, as browsers write it.
func sdp(lines ...string) string {
	return strings.Join(append([]string{"v=0", "o=- 4611731400430051336 2 IN IP4 127.0.0.1", "s=-", "t=0 0"}, lines...), "\r\n") + "\r\n"
}

func TestCheckSDP(t *testing.T) {
	audio := "m=audio 9 UDP/TLS/RTP/SAVPF 111"
	tests := []struct {
		name       string
		sdp        string
		mayPublish bool
		want       error
	}{
		{"speaker sendrecv", sdp(audio, "a=mid:0", "a=sendrecv", "a=rtpmap:111 opus/48000/2"), true, nil},
		{"speaker without a direction", sdp(audio, "a=mid:0"), true, nil},
		{"speaker sendonly", sdp(audio, "a=sendonly"), true, nil},
		{"two audio sections", sdp(audio, "a=sendrecv", audio, "a=recvonly"), true, nil},
		{"listener recvonly", sdp(audio, "a=mid:0", "a=recvonly"), false, nil},
		{"listener inactive", sdp(audio, "a=inactive"), false, nil},
		{"listener with a recvonly session level", sdp("a=recvonly", audio, "a=mid:0"), false, nil},
		{"listener with LF line breaks", strings.ReplaceAll(sdp(audio, "a=recvonly"), "\r\n", "\n"), false, nil},
		{"no media section", sdp(), false, nil},
		{"listener sendrecv", sdp(audio, "a=sendrecv"), false, ErrSDPDirection},
		{"listener sendonly", sdp(audio, "a=sendonly"), false, ErrSDPDirection},
		{"listener without a direction", sdp(audio, "a=mid:0"), false, ErrSDPDirection},
		{"listener overriding a recvonly session level", sdp("a=recvonly", audio, "a=sendrecv"), false, ErrSDPDirection},
		{"listener with one sending section", sdp(audio, "a=recvonly", audio, "a=sendrecv"), false, ErrSDPDirection},
		{"video", sdp(audio, "a=sendrecv", "m=video 9 UDP/TLS/RTP/SAVPF 96", "a=sendrecv"), true, ErrSDPMedia},
		{"data channel", sdp("m=application 9 UDP/DTLS/SCTP webrtc-datachannel", "a=sctp-port:5000"), true, ErrSDPMedia},
		{"video of a listener", sdp("m=video 9 UDP/TLS/RTP/SAVPF 96", "a=recvonly"), false, ErrSDPMedia},
		{"no version line", "m=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n", true, ErrSDPMalformed},
		{"empty", "", true, ErrSDPMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckSDP(tt.sdp, tt.mayPublish); !errors.Is(err, tt.want) || (tt.want == nil && err != nil) {
				t.Errorf("CheckSDP = %v, want %v", err, tt.want)
			}
		})
	}
}
