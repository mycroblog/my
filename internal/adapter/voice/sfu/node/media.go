// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

// opusFmtp turns on Opus DTX and in-band FEC (ADR-0101 §12); minptime is what the browsers send.
const opusFmtp = "minptime=10;useinbandfec=1;usedtx=1"

// opusPayloadType is the payload type of Opus in the node's offers, the one the browsers use.
const opusPayloadType = 111

// gatherTimeout bounds the wait for the candidates of an offer: an ICE-lite node on its muxes
// has them at once.
const gatherTimeout = 2 * time.Second

// tcpReadBuffer is the buffer of the passive ICE-TCP mux, in packets.
const tcpReadBuffer = 8

// media is the WebRTC API of a node and the sockets that its connections share.
type media struct {
	api *webrtc.API
	udp net.PacketConn
	tcp net.Listener
	// own reports sockets that the node opened and closes.
	own bool
}

// newMedia opens the sockets of o and returns the API on them (ADR-0101 §15): ICE-lite, one UDP
// port through a mux and passive ICE-TCP on one port, the public IP as the host candidate when
// set, Opus only with the audio level extension, and no NACK, RTX or data channel: only the RTCP
// reports.
func newMedia(ctx context.Context, o Options, logger *slog.Logger) (*media, error) {
	m := &media{udp: o.UDPConn, tcp: o.TCPListener}
	if m.udp == nil || m.tcp == nil {
		var lc net.ListenConfig
		udp, err := lc.ListenPacket(ctx, "udp", fmt.Sprintf(":%d", o.UDPPort))
		if err != nil {
			return nil, fmt.Errorf("voice_sfu_udp_port: %w", err)
		}
		tcp, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", o.TCPPort))
		if err != nil {
			_ = udp.Close()
			return nil, fmt.Errorf("voice_sfu_tcp_port: %w", err)
		}
		m.udp, m.tcp, m.own = udp, tcp, true
	}
	logs := pionLogs{logger: logger}
	var se webrtc.SettingEngine
	se.LoggerFactory = logs
	se.SetLite(true)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6, webrtc.NetworkTypeTCP4, webrtc.NetworkTypeTCP6})
	se.SetICEUDPMux(webrtc.NewICEUDPMux(logs.NewLogger("udpmux"), m.udp))
	se.SetICETCPMux(webrtc.NewICETCPMux(logs.NewLogger("tcpmux"), m.tcp, tcpReadBuffer))
	se.DisableActiveTCP(true)
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	se.SetIncludeLoopbackCandidate(o.Loopback)
	if o.PublicIP != "" {
		rule := webrtc.ICEAddressRewriteRule{External: []string{o.PublicIP}, AsCandidateType: webrtc.ICECandidateTypeHost, Mode: webrtc.ICEAddressRewriteReplace}
		if err := se.SetICEAddressRewriteRules(rule); err != nil {
			m.close()
			return nil, fmt.Errorf("voice_sfu_public_ip: %w", err)
		}
	}
	engine := &webrtc.MediaEngine{}
	if err := RegisterCodecs(engine); err != nil {
		m.close()
		return nil, err
	}
	reg := &interceptor.Registry{}
	if err := webrtc.ConfigureRTCPReports(reg); err != nil {
		m.close()
		return nil, err
	}
	m.api = webrtc.NewAPI(webrtc.WithSettingEngine(se), webrtc.WithMediaEngine(engine), webrtc.WithInterceptorRegistry(reg))
	return m, nil
}

// RegisterCodecs registers what a node negotiates on engine: Opus with DTX and in-band FEC, and
// the audio level header extension (RFC 6464). A simulated client registers the same.
func RegisterCodecs(engine *webrtc.MediaEngine) error {
	opus := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: opusFmtp},
		PayloadType:        opusPayloadType,
	}
	if err := engine.RegisterCodec(opus, webrtc.RTPCodecTypeAudio); err != nil {
		return err
	}
	return engine.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: sdp.AudioLevelURI}, webrtc.RTPCodecTypeAudio)
}

// close closes the sockets the node opened.
func (m *media) close() {
	if !m.own {
		return
	}
	_ = m.udp.Close()
	_ = m.tcp.Close()
}

// pionLogs sends the errors of Pion to the node's log as warnings and its warnings as debug
// lines, which a closing connection writes in numbers; the rest is dropped.
type pionLogs struct{ logger *slog.Logger }

func (l pionLogs) NewLogger(scope string) logging.LeveledLogger {
	return pionLogger{logger: l.logger.With(slog.String("pion", scope))}
}

type pionLogger struct{ logger *slog.Logger }

func (pionLogger) Trace(string)          {}
func (pionLogger) Tracef(string, ...any) {}
func (pionLogger) Debug(string)          {}
func (pionLogger) Debugf(string, ...any) {}
func (pionLogger) Info(string)           {}
func (pionLogger) Infof(string, ...any)  {}
func (l pionLogger) Warn(msg string)     { l.logger.Debug(msg) }
func (l pionLogger) Warnf(format string, args ...any) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}
func (l pionLogger) Error(msg string) { l.logger.Warn(msg) }
func (l pionLogger) Errorf(format string, args ...any) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}
