// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// The packet path (ADR-0101 §12). A speaker's packets are read once into a pooled buffer, checked
// (may the session speak, is it sound), and handed to the bounded queue of every subscriber that
// has a slot for the speaker; each subscriber's writer rewrites the sequence numbers and
// timestamps of its slot and writes. Nothing on the path takes a lock of the room: the
// subscribers and the slots are copy-on-write lists behind atomic pointers.

const (
	// maxPacketBytes is the largest RTP packet read: an Opus packet is far smaller.
	maxPacketBytes = 1500
	// QueueLength bounds the packets that wait for one subscriber's writer: about a second of
	// five speakers. A packet that finds it full is dropped and counted (queue_full).
	QueueLength = 256
	// SilentLevel is the audio level (RFC 6464, -dBov) from which a packet is silence and is
	// dropped: below any voice, above the comfort noise of DTX.
	SilentLevel = 100
	// SpeakingLevel is the audio level up to which a packet is someone speaking, for speaking.
	SpeakingLevel = 60
	// opusClockRate is the RTP clock rate of Opus, in ticks per second.
	opusClockRate = 48000
)

// The reasons of a dropped packet (Packets).
const (
	DropSilent     = "silent"
	DropNotAllowed = "not_allowed"
	DropQueueFull  = "queue_full"
)

// packet is a received RTP packet shared by the subscribers it goes to, released to its pool by
// the last one.
type packet struct {
	buf    [maxPacketBytes]byte
	rtp    rtp.Packet
	source *source
	// seq is the packet's sequence number less the packets of its source dropped before it, so
	// that the drops leave no gap (they look like DTX to the receiver), while a loss on the way to
	// the node stays one.
	seq  uint16
	refs atomic.Int32
	pool *sync.Pool
}

func newPacketPool() *sync.Pool {
	p := &sync.Pool{}
	p.New = func() any { return &packet{pool: p} }
	return p
}

func (p *packet) release() {
	if p.refs.Add(-1) == 0 {
		p.source = nil
		p.pool.Put(p)
	}
}

// source is the ingress of one session's microphone.
type source struct {
	// allowed reports a session that may speak and is not muted; its packets are dropped
	// otherwise (ADR-0101 §12).
	allowed atomic.Bool
	// loud is when the session last sent a packet louder than SpeakingLevel, in Unix nanoseconds.
	loud atomic.Int64
	// skipped counts the packets dropped at the source, for packet.seq; only the reader of the
	// source's track touches it.
	skipped uint16
	// levelID is the ID of the audio level extension of the source's track, 0 without one.
	levelID uint8
}

// audioLevel returns the audio level of a packet (RFC 6464: 0 is loudest, 127 silence), found
// false when it has none.
func audioLevel(h *rtp.Header, id uint8) (uint8, bool) {
	if id == 0 || !h.Extension {
		return 0, false
	}
	b := h.GetExtension(id)
	if len(b) == 0 {
		return 0, false
	}
	return b[0] & 0x7f, true
}

// delivery is a packet for a slot of a subscriber.
type delivery struct {
	slot *slotTrack
	pkt  *packet
}

// subscriber is the egress of one session: the slots that carry the speakers to it, and its
// writer's queue.
type subscriber struct {
	// slots maps each source that the session hears to the slot that carries it.
	slots atomic.Pointer[map[*source]*slotTrack]
	queue chan delivery
}

func newSubscriber() *subscriber {
	s := &subscriber{queue: make(chan delivery, QueueLength)}
	s.slots.Store(&map[*source]*slotTrack{})
	return s
}

// forwarder carries the packets of the sources of a room to its subscribers.
type forwarder struct {
	subs  atomic.Pointer[[]*subscriber]
	pool  *sync.Pool
	stats *stats
	now   func() time.Time
}

func newForwarder(st *stats, now func() time.Time) *forwarder {
	f := &forwarder{pool: newPacketPool(), stats: st, now: now}
	f.subs.Store(&[]*subscriber{})
	return f
}

// setSubscribers replaces the subscribers of the room.
func (f *forwarder) setSubscribers(subs []*subscriber) { f.subs.Store(&subs) }

// receive takes a packet that src received into pkt.buf[:n] and hands it to the subscribers.
// It is called by the one reader of src.
func (f *forwarder) receive(src *source, pkt *packet, n int) {
	if err := pkt.rtp.Unmarshal(pkt.buf[:n]); err != nil {
		pkt.refs.Store(1)
		pkt.release()
		return
	}
	pkt.refs.Store(1)
	defer pkt.release()
	if !src.allowed.Load() {
		src.skipped++
		f.stats.dropped(DropNotAllowed)
		return
	}
	if level, ok := audioLevel(&pkt.rtp.Header, src.levelID); ok {
		if level >= SilentLevel {
			src.skipped++
			f.stats.dropped(DropSilent)
			return
		}
		if level <= SpeakingLevel {
			src.loud.Store(f.now().UnixNano())
		}
	}
	pkt.source, pkt.seq = src, pkt.rtp.SequenceNumber-src.skipped
	for _, s := range *f.subs.Load() {
		slot := (*s.slots.Load())[src]
		if slot == nil {
			continue
		}
		pkt.refs.Add(1)
		select {
		case s.queue <- delivery{slot: slot, pkt: pkt}:
		default:
			pkt.release()
			f.stats.dropped(DropQueueFull)
		}
	}
}

// read reads the packets of a remote track into the forwarder until it ends.
func (f *forwarder) read(src *source, track *webrtc.TrackRemote) {
	for {
		pkt := f.pool.Get().(*packet)
		n, _, err := track.Read(pkt.buf[:])
		if err != nil {
			pkt.refs.Store(1)
			pkt.release()
			return
		}
		f.receive(src, pkt, n)
	}
}

// write runs the writer of a subscriber until done closes.
func (f *forwarder) write(s *subscriber, done <-chan struct{}) {
	for {
		select {
		case d := <-s.queue:
			if n, ok := d.slot.write(d.pkt, f.now); ok {
				f.stats.forwarded(n)
			}
			d.pkt.release()
		case <-done:
			for {
				select {
				case d := <-s.queue:
					d.pkt.release()
				default:
					return
				}
			}
		}
	}
}

// binding is where a slot writes: the sender of its transceiver.
type binding struct {
	ssrc        uint32
	payloadType uint8
	writer      webrtc.TrackLocalWriter
}

// slotTrack is the outgoing track of a transceiver that carries one speaker at a time to a
// subscriber (webrtc.TrackLocal). When its speaker changes, its sequence numbers and timestamps
// go on from where they were, so that the receiver sees one stream.
type slotTrack struct {
	id      string
	binding atomic.Pointer[binding]

	// The state of the rewriting, which only the subscriber's writer touches.
	started bool
	from    *source
	fromSSR uint32
	seqOff  uint16
	tsOff   uint32
	lastSeq uint16
	lastTS  uint32
	lastAt  time.Time
	// header is the header that write rewrites and writes, kept so that a packet costs no
	// allocation: the writer of the sender reads it during WriteRTP only.
	header rtp.Header
}

var _ webrtc.TrackLocal = (*slotTrack)(nil)

func newSlotTrack(id string) *slotTrack { return &slotTrack{id: id} }

// Bind implements webrtc.TrackLocal: the slot writes Opus to the sender of the transceiver.
func (t *slotTrack) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	for _, c := range ctx.CodecParameters() {
		if strings.EqualFold(c.MimeType, webrtc.MimeTypeOpus) {
			t.binding.Store(&binding{ssrc: uint32(ctx.SSRC()), payloadType: uint8(c.PayloadType), writer: ctx.WriteStream()})
			return c, nil
		}
	}
	return webrtc.RTPCodecParameters{}, webrtc.ErrUnsupportedCodec
}

// Unbind implements webrtc.TrackLocal.
func (t *slotTrack) Unbind(webrtc.TrackLocalContext) error {
	t.binding.Store(nil)
	return nil
}

// ID implements webrtc.TrackLocal.
func (t *slotTrack) ID() string { return t.id }

// RID implements webrtc.TrackLocal.
func (*slotTrack) RID() string { return "" }

// StreamID implements webrtc.TrackLocal.
func (t *slotTrack) StreamID() string { return t.id }

// Kind implements webrtc.TrackLocal.
func (*slotTrack) Kind() webrtc.RTPCodecType { return webrtc.RTPCodecTypeAudio }

// write writes a packet to the slot's sender and returns the bytes written, ok false when the
// slot is not bound.
func (t *slotTrack) write(pkt *packet, now func() time.Time) (int, bool) {
	b := t.binding.Load()
	if b == nil {
		return 0, false
	}
	at := now()
	in := &pkt.rtp.Header
	if !t.started || t.from != pkt.source || t.fromSSR != in.SSRC {
		// A new speaker, or the same one on a new stream: go on from the last packet, with the
		// time that passed since.
		ticks := uint32(1)
		if t.started {
			// RTP timestamps wrap modulo 2^32 (RFC 3550 §5.1), so the truncation is the arithmetic.
			ticks = max(uint32(at.Sub(t.lastAt)*opusClockRate/time.Second), 1) //nolint:gosec // wraps by design
		}
		t.seqOff = t.lastSeq + 1 - pkt.seq
		t.tsOff = t.lastTS + ticks - in.Timestamp
		t.started, t.from, t.fromSSR = true, pkt.source, in.SSRC
	}
	h := &t.header
	*h = *in
	h.SSRC, h.PayloadType = b.ssrc, b.payloadType
	h.SequenceNumber = pkt.seq + t.seqOff
	h.Timestamp = in.Timestamp + t.tsOff
	h.Padding, h.PaddingSize = false, 0
	t.lastSeq, t.lastTS, t.lastAt = h.SequenceNumber, h.Timestamp, at
	n, err := b.writer.WriteRTP(h, pkt.rtp.Payload)
	if err != nil {
		// The connection closed meanwhile.
		return 0, false
	}
	return n, true
}
