// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// capture is a TrackLocalWriter that keeps the headers it is given.
type capture struct {
	mu      sync.Mutex
	headers []rtp.Header
}

func (c *capture) WriteRTP(h *rtp.Header, p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.headers = append(c.headers, h.Clone())
	return h.MarshalSize() + len(p), nil
}

func (c *capture) Write(b []byte) (int, error) { return len(b), nil }

// audioPacket returns the bytes of an RTP packet with an audio level.
func audioPacket(t testing.TB, ssrc uint32, seq uint16, ts uint32, level uint8) []byte {
	t.Helper()
	p := rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 111, SSRC: ssrc, SequenceNumber: seq, Timestamp: ts}, Payload: []byte{1, 2, 3}}
	if err := p.SetExtension(1, []byte{level}); err != nil {
		t.Fatal(err)
	}
	b, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestRewritesTheSequence drops silent packets without a gap, keeps a loss on the way, and goes
// on from the last packet when the slot carries another speaker.
func TestRewritesTheSequence(t *testing.T) {
	f := newForwarder(newStats(nil), time.Now)
	a, b := &source{levelID: 1}, &source{levelID: 1}
	a.allowed.Store(true)
	b.allowed.Store(true)
	w := &capture{}
	slot := newSlotTrack("slot")
	slot.binding.Store(&binding{ssrc: 7, payloadType: 111, writer: w})
	sub := newSubscriber()
	sub.slots.Store(&map[*source]*slotTrack{a: slot})
	f.setSubscribers([]*subscriber{sub})
	send := func(src *source, ssrc uint32, seq uint16, level uint8) {
		pkt := f.pool.Get().(*packet)
		n := copy(pkt.buf[:], audioPacket(t, ssrc, seq, uint32(seq)*960, level))
		f.receive(src, pkt, n)
		for len(sub.queue) > 0 {
			d := <-sub.queue
			d.slot.write(d.pkt, time.Now)
			d.pkt.release()
		}
	}
	send(a, 100, 10, 30)
	send(a, 100, 11, 127) // silent: dropped
	send(a, 100, 12, 30)
	send(a, 100, 14, 30) // 13 was lost on the way
	sub.slots.Store(&map[*source]*slotTrack{b: slot})
	send(b, 200, 500, 30)
	send(b, 200, 501, 30)
	var got []uint16
	for _, h := range w.headers {
		if h.SSRC != 7 || h.PayloadType != 111 {
			t.Errorf("header %+v, want the slot's SSRC and payload type", h)
		}
		got = append(got, h.SequenceNumber)
	}
	first := got[0]
	want := []uint16{first, first + 1, first + 3, first + 4, first + 5}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("sequence numbers = %v, want %v", got, want)
	}
	if tot := f.stats.totals(); tot.Silent != 1 {
		t.Errorf("totals = %+v, want 1 silent", tot)
	}
}

// discard is a TrackLocalWriter that writes nothing.
type discard struct{}

func (discard) WriteRTP(h *rtp.Header, p []byte) (int, error) { return h.MarshalSize() + len(p), nil }
func (discard) Write(b []byte) (int, error)                   { return len(b), nil }

// BenchmarkForward forwards the packets of one speaker to n subscribers. One op is a packet
// received, checked, queued for every subscriber, and rewritten and written by each subscriber's
// slot, drained in the same goroutine so that no packet is dropped for a full queue: ns/forward is
// the cost of one forwarded packet on the path of ADR-0101 §12, without SRTP and the socket.
func BenchmarkForward(b *testing.B) {
	for _, n := range []int{1, 10, 50, 100} {
		b.Run(fmt.Sprintf("subscribers=%d", n), func(b *testing.B) {
			f := newForwarder(newStats(nil), time.Now)
			src := &source{levelID: 1}
			src.allowed.Store(true)
			subs := make([]*subscriber, n)
			for i := range subs {
				s := newSubscriber()
				slot := newSlotTrack("slot")
				slot.binding.Store(&binding{ssrc: uint32(i), payloadType: 111, writer: discard{}})
				s.slots.Store(&map[*source]*slotTrack{src: slot})
				subs[i] = s
			}
			f.setSubscribers(subs)
			raw := audioPacket(b, 100, 0, 0, 30)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				pkt := f.pool.Get().(*packet)
				m := copy(pkt.buf[:], raw)
				pkt.buf[2], pkt.buf[3] = byte(i>>8), byte(i)
				f.receive(src, pkt, m)
				for _, s := range subs {
					d := <-s.queue
					if _, ok := d.slot.write(d.pkt, f.now); !ok {
						b.Fatal("the slot did not write")
					}
					d.pkt.release()
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/forward")
			if tot := f.stats.totals(); tot.QueueFull != 0 {
				b.Errorf("totals = %+v, want no packet dropped", tot)
			}
		})
	}
}
