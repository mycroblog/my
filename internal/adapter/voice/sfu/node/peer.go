// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"

	"github.com/mycroblog/my/internal/app/kernel"
)

const (
	// ReconnectGrace is how long a connection waits for its session's WebSocket to come back
	// (a new ticket for the same session, ADR-0101 §11) before it closes.
	ReconnectGrace = 30 * time.Second
	// ConnectTimeout is how long a new connection may take to connect before it closes.
	ConnectTimeout = 30 * time.Second
	// statsInterval is how often a connection records the RTT and loss of its client.
	statsInterval = 5 * time.Second
	// ntpEpochOffset is the seconds from 1900 (NTP) to 1970 (Unix).
	ntpEpochOffset = 2208988800
)

// peer is the one PeerConnection of a session on the node (ADR-0101 §11): a transceiver that
// receives the user's microphone, and one slot per speaker that sends that speaker's audio. The
// node always makes the offers.
type peer struct {
	room    *room
	session string
	account string
	src     source
	sub     *subscriber
	pc      *webrtc.PeerConnection
	done    chan struct{}
	closed  atomic.Bool
	statsAt atomic.Int64
	// connected reports a connection that connected once; until then connectTimer closes it at
	// ConnectTimeout.
	connected    atomic.Bool
	connectTimer *time.Timer

	// canSpeak is whether the session is a speaker, under room.mu.
	canSpeak bool

	mu          sync.Mutex
	ws          *wsConn
	slots       []*slot
	offered     bool
	negotiating bool
	// pending is an offer to make after the answer to the current one, restart whether it
	// restarts ICE.
	pending, restart bool
	grace            *time.Timer
	migrated         bool
}

// slot is a transceiver that sends one speaker, or nobody while speaker is nil.
type slot struct {
	tr      *webrtc.RTPTransceiver
	track   *slotTrack
	speaker *peer
}

// newPeer makes the PeerConnection of a session of r.
func newPeer(r *room, session, account string) (*peer, error) {
	n := r.node
	pc, err := n.media.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, err
	}
	p := &peer{room: r, session: session, account: account, sub: newSubscriber(), pc: pc, done: make(chan struct{})}
	p.connectTimer = time.AfterFunc(ConnectTimeout, func() {
		if !p.connected.Load() {
			p.close(CodeRoomUnavailable, "the connection did not connect")
		}
	})
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		p.connectTimer.Stop()
		_ = pc.Close()
		return nil, err
	}
	pc.OnTrack(func(track *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
		p.src.levelID = audioLevelID(recv)
		n.run("voice.sfu.read", func() { r.fwd.read(&p.src, track) })
		n.run("voice.sfu.read_rtcp", func() { drainRTCP(recv) })
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		switch s {
		case webrtc.PeerConnectionStateConnected:
			p.connected.Store(true)
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			p.close(CodeRoomUnavailable, "the connection failed")
		}
	})
	n.run("voice.sfu.write", func() { r.fwd.write(p.sub, p.done) })
	return p, nil
}

// audioLevelID returns the ID of the audio level extension of a receiver, 0 without one.
func audioLevelID(recv *webrtc.RTPReceiver) uint8 {
	for _, e := range recv.GetParameters().HeaderExtensions {
		if e.URI == sdp.AudioLevelURI {
			return uint8(e.ID) //nolint:gosec // an extension ID is 1 to 255 (RFC 8285)
		}
	}
	return 0
}

// drainRTCP reads the RTCP of the microphone, which the report interceptors use, until it ends.
func drainRTCP(recv *webrtc.RTPReceiver) {
	for {
		if _, _, err := recv.ReadRTCP(); err != nil {
			return
		}
	}
}

// attach makes ws the session's WebSocket, replacing the one before, and sends what it missed:
// the offer that waits for an answer, or the one that waited for a WebSocket.
func (p *peer) attach(ws *wsConn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed.Load() {
		ws.send(errorMessage(CodeRoomUnavailable, "the session left the node"))
		ws.close(websocket.StatusNormalClosure, "")
		return
	}
	if old := p.ws; old != nil {
		old.close(websocket.StatusNormalClosure, "replaced")
	}
	p.ws, p.migrated = ws, false
	if p.grace != nil {
		p.grace.Stop()
		p.grace = nil
	}
	switch {
	case p.negotiating:
		p.sendOfferLocked()
	case p.pending:
		p.negotiateLocked(p.restart)
	}
}

// detach forgets ws when it is still the session's WebSocket, and closes the connection when no
// WebSocket comes back within ReconnectGrace.
func (p *peer) detach(ws *wsConn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ws != ws {
		return
	}
	p.ws = nil
	p.waitLocked()
}

func (p *peer) waitLocked() {
	if p.grace == nil {
		p.grace = time.AfterFunc(ReconnectGrace, func() { p.close("", "") })
	}
}

// send sends m on the session's WebSocket, when it has one.
func (p *peer) send(m Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ws != nil {
		p.ws.send(m)
	}
}

// setSpeakers makes the slots of the session carry want, the speakers it hears, and offers again
// when they changed (or for the first offer). A slot of a speaker who stopped carries the next new
// one; a new transceiver is added only when no slot is free.
func (p *peer) setSpeakers(want []*peer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed.Load() {
		return
	}
	changed := false
	for _, s := range p.slots {
		if s.speaker != nil && !slices.Contains(want, s.speaker) {
			s.speaker, changed = nil, true
		}
	}
	for _, sp := range want {
		if slices.ContainsFunc(p.slots, func(s *slot) bool { return s.speaker == sp }) {
			continue
		}
		i := slices.IndexFunc(p.slots, func(s *slot) bool { return s.speaker == nil })
		if i < 0 {
			s, err := p.newSlotLocked()
			if err != nil {
				p.room.node.o.Logger.Warn("a voice chat media node could not add a transceiver", "error", err)
				continue
			}
			i = len(p.slots)
			p.slots = append(p.slots, s)
		}
		p.slots[i].speaker, changed = sp, true
	}
	if changed {
		m := make(map[*source]*slotTrack, len(want))
		for _, s := range p.slots {
			if s.speaker != nil {
				m[&s.speaker.src] = s.track
			}
		}
		p.sub.slots.Store(&m)
	}
	if changed || !p.offered {
		p.negotiateLocked(false)
	}
}

// newSlotLocked adds a transceiver that sends to the session.
func (p *peer) newSlotLocked() (*slot, error) {
	track := newSlotTrack(fmt.Sprintf("speaker-%d", len(p.slots)))
	tr, err := p.pc.AddTransceiverFromTrack(track, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
	if err != nil {
		return nil, err
	}
	sender := tr.Sender()
	p.room.node.run("voice.sfu.read_rtcp", func() { p.readRTCP(sender, track) })
	return &slot{tr: tr, track: track}, nil
}

// negotiateLocked makes an offer, or remembers it until the answer to the current one or until
// the session has a WebSocket.
func (p *peer) negotiateLocked(restart bool) {
	if p.negotiating || p.ws == nil {
		p.pending, p.restart = true, p.restart || restart
		return
	}
	p.pending, p.restart = false, false
	offer, err := p.pc.CreateOffer(&webrtc.OfferOptions{ICERestart: restart})
	if err != nil {
		p.room.node.o.Logger.Warn("a voice chat media node could not make an offer", "error", err)
		return
	}
	gathered := webrtc.GatheringCompletePromise(p.pc)
	if err := p.pc.SetLocalDescription(offer); err != nil {
		p.room.node.o.Logger.Warn("a voice chat media node could not set its offer", "error", err)
		return
	}
	select {
	case <-gathered:
	case <-time.After(gatherTimeout):
	}
	st := p.room.node.stats
	if p.offered {
		st.renegotiations.Add(p.room.node.base, 1, kernel.Labels{})
	}
	if restart {
		st.restarts.Add(p.room.node.base, 1, kernel.Labels{})
	}
	p.offered, p.negotiating = true, true
	p.sendOfferLocked()
}

// sendOfferLocked sends the current offer with the speaker of each slot.
func (p *peer) sendOfferLocked() {
	ld := p.pc.LocalDescription()
	if ld == nil || p.ws == nil {
		return
	}
	tracks := []Track{}
	for _, s := range p.slots {
		if s.speaker != nil && s.tr.Mid() != "" {
			tracks = append(tracks, Track{Mid: s.tr.Mid(), Participant: s.speaker.account})
		}
	}
	p.ws.send(Message{V: Version, Type: TypeOffer, SDP: ld.SDP, Tracks: tracks})
}

// answer applies the client's answer to the current offer, and makes the offer that waited for it.
func (p *peer) answer(ws *wsConn, sdpText string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.negotiating {
		return // an answer to an offer that a newer one replaced
	}
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdpText}); err != nil {
		ws.send(errorMessage(CodeInvalidMessage, "the answer does not apply: "+err.Error()))
		ws.close(websocket.StatusPolicyViolation, "invalid answer")
		return
	}
	p.negotiating = false
	if p.pending {
		p.negotiateLocked(p.restart)
	}
}

// candidate adds an ICE candidate of the client: the JSON of an RTCIceCandidateInit.
func (p *peer) candidate(ws *wsConn, text string) {
	var c webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(text), &c); err != nil {
		ws.send(errorMessage(CodeInvalidMessage, "the candidate is not the JSON of an RTCIceCandidateInit"))
		ws.close(websocket.StatusPolicyViolation, "invalid candidate")
		return
	}
	if c.Candidate == "" {
		return // the end of the candidates
	}
	_ = p.pc.AddICECandidate(c)
}

// restartICE makes an offer that restarts ICE, after the client's network changed.
func (p *peer) restartICE() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.negotiateLocked(true)
}

// migrate tells the client that its room moved to another node, and closes the WebSocket; the
// connection lasts until the client leaves it or ReconnectGrace passes (ADR-0101 §14).
func (p *peer) migrate() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.migrated || p.ws == nil {
		return false
	}
	p.migrated = true
	p.ws.send(Message{V: Version, Type: TypeMigrate})
	p.ws.close(websocket.StatusNormalClosure, "migrate")
	p.ws = nil
	p.waitLocked()
	return true
}

// close ends the session on the node, telling the client why with code unless it is empty.
func (p *peer) close(code, msg string) {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	p.connectTimer.Stop()
	p.mu.Lock()
	ws := p.ws
	p.ws = nil
	if p.grace != nil {
		p.grace.Stop()
	}
	p.mu.Unlock()
	if ws != nil {
		if code != "" {
			ws.send(errorMessage(code, msg))
		}
		ws.close(websocket.StatusNormalClosure, "")
	}
	close(p.done)
	p.room.leave(p)
	_ = p.pc.Close()
}

// readRTCP reads the RTCP of a slot's sender: the receiver reports of the client give its RTT
// and loss.
func (p *peer) readRTCP(sender *webrtc.RTPSender, track *slotTrack) {
	for {
		pkts, _, err := sender.ReadRTCP()
		if err != nil {
			return
		}
		for _, pkt := range pkts {
			rr, ok := pkt.(*rtcp.ReceiverReport)
			if !ok {
				continue
			}
			b := track.binding.Load()
			for _, rep := range rr.Reports {
				if b != nil && rep.SSRC == b.ssrc {
					p.report(rep)
				}
			}
		}
	}
}

// report records the RTT and loss of a receiver report, once per statsInterval of the session.
func (p *peer) report(rep rtcp.ReceptionReport) {
	now := time.Now()
	last := p.statsAt.Load()
	if now.UnixNano()-last < int64(statsInterval) || !p.statsAt.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	st, ctx := p.room.node.stats, p.room.node.base
	st.loss.Record(ctx, float64(rep.FractionLost)*100/256, kernel.Labels{})
	if rep.LastSenderReport == 0 {
		return
	}
	// RFC 3550 §6.4.1: the arrival less the time of the sender report and the delay since.
	rtt := float64(ntpCompact(now)-rep.LastSenderReport-rep.Delay) / 65536
	if rtt >= 0 && rtt < 60 {
		st.rtt.Record(ctx, rtt, kernel.Labels{})
	}
}

// ntpCompact returns the middle 32 bits of the NTP time of t.
func ntpCompact(t time.Time) uint32 {
	// NTP time wraps modulo 2^32 seconds, so the truncations are the arithmetic.
	secs := uint64(t.Unix()) + ntpEpochOffset                  //nolint:gosec // wraps by design
	frac := uint64(t.Nanosecond()) << 32 / uint64(time.Second) //nolint:gosec // below a second
	return uint32((secs<<32 | frac) >> 16)                     //nolint:gosec // the middle 32 bits

}
