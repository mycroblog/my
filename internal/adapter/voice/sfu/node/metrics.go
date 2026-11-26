// SPDX-License-Identifier: AGPL-3.0-only

package node

import (
	"context"
	"sync/atomic"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The metrics of a media node (ADR-0055, ADR-0101 §16). None has a room or session label: the
// resource attributes of the process tell the nodes apart.
var (
	Rooms = kernel.Instrument{
		Name: "my.voice.sfu.rooms", Unit: "{room}",
		Description: "Rooms that the voice chat media node holds connections of.",
	}
	Sessions = kernel.Instrument{
		Name: "my.voice.sfu.sessions", Unit: "{session}",
		Description: "Sessions connected to the voice chat media node.",
	}
	CapacityReserved = kernel.Instrument{
		Name: "my.voice.sfu.capacity.reserved", Unit: "{participant}",
		Description: "The sum of the maximums of the rooms placed on the voice chat media node.",
	}
	CapacityConnected = kernel.Instrument{
		Name: "my.voice.sfu.capacity.connected", Unit: "{participant}",
		Description: "The sessions connected to the voice chat media node, against its capacity.",
	}
	// Packets counts the RTP packets of the node by outcome: forwarded (one per subscriber
	// written to), or dropped as silent, not_allowed (a session that may not speak or is muted),
	// or queue_full (a subscriber's queue was full).
	Packets = kernel.Instrument{
		Name: "my.voice.sfu.packets", Unit: "{packet}",
		Description: "RTP packets of the voice chat media node, forwarded or dropped by reason.",
		LabelKeys:   []string{"my.outcome"},
	}
	// Bytes counts the bytes of the forwarded RTP packets.
	Bytes = kernel.Instrument{
		Name: "my.voice.sfu.bytes", Unit: "By",
		Description: "Bytes of the RTP packets that the voice chat media node forwarded.",
	}
	Renegotiations = kernel.Instrument{
		Name: "my.voice.sfu.renegotiations", Unit: "{offer}",
		Description: "Offers of the voice chat media node after the first of a connection.",
	}
	ICERestarts = kernel.Instrument{
		Name: "my.voice.sfu.ice_restarts", Unit: "{restart}",
		Description: "ICE restarts that the clients of the voice chat media node asked for.",
	}
	Migrations = kernel.Instrument{
		Name: "my.voice.sfu.migrations", Unit: "{session}",
		Description: "Sessions that a draining voice chat media node sent to another node.",
	}
	// TicketRefusals counts the connections refused for their ticket, by reason: invalid (expired,
	// used, or unknown) or other_node.
	TicketRefusals = kernel.Instrument{
		Name: "my.voice.sfu.ticket_refusals", Unit: "{ticket}",
		Description: "Connections to the voice chat media node refused for their ticket.",
		LabelKeys:   []string{"my.voice.sfu.reason"},
	}
	ClientRTT = kernel.Instrument{
		Name: "my.voice.sfu.client.rtt", Unit: "s",
		Description: "Round-trip time to the clients of the voice chat media node, from their RTCP receiver reports.",
		Buckets:     []float64{0.01, 0.025, 0.05, 0.075, 0.1, 0.15, 0.2, 0.3, 0.5, 1, 2},
	}
	ClientPacketLoss = kernel.Instrument{
		Name: "my.voice.sfu.client.packet_loss", Unit: "%",
		Description: "Packet loss on the way to the clients of the voice chat media node, from their RTCP receiver reports.",
		Buckets:     []float64{0.5, 1, 2, 5, 10, 20, 50},
	}
)

// The outcomes of Packets besides the drop reasons.
const OutcomeForwarded = "forwarded"

// The reasons of TicketRefusals.
const (
	RefusalInvalid   = "invalid"
	RefusalOtherNode = "other_node"
)

// stats keeps the totals of the packet path in counters of its own, which the meter reads when
// it collects, so that a packet costs an atomic add.
type stats struct {
	forwardedPackets, forwardedBytes atomic.Uint64
	silent, notAllowed, queueFull    atomic.Uint64
	renegotiations, restarts         kernel.Counter
	migrations, refusals             kernel.Counter
	rtt, loss                        kernel.Histogram
}

func newStats(m kernel.Meter) *stats {
	m = kernel.MeterOrNop(m)
	s := &stats{
		renegotiations: m.Counter(Renegotiations), restarts: m.Counter(ICERestarts),
		migrations: m.Counter(Migrations), refusals: m.Counter(TicketRefusals),
		rtt: m.Histogram(ClientRTT), loss: m.Histogram(ClientPacketLoss),
	}
	m.ObservableCounter(Packets, func(_ context.Context, o kernel.GaugeObserver) error {
		o.Observe(float64(s.forwardedPackets.Load()), kernel.NewLabels("my.outcome", OutcomeForwarded))
		o.Observe(float64(s.silent.Load()), kernel.NewLabels("my.outcome", DropSilent))
		o.Observe(float64(s.notAllowed.Load()), kernel.NewLabels("my.outcome", DropNotAllowed))
		o.Observe(float64(s.queueFull.Load()), kernel.NewLabels("my.outcome", DropQueueFull))
		return nil
	})
	m.ObservableCounter(Bytes, func(_ context.Context, o kernel.GaugeObserver) error {
		o.Observe(float64(s.forwardedBytes.Load()), kernel.Labels{})
		return nil
	})
	return s
}

func (s *stats) forwarded(n int) {
	s.forwardedPackets.Add(1)
	s.forwardedBytes.Add(uint64(max(n, 0)))
}

func (s *stats) dropped(reason string) {
	switch reason {
	case DropSilent:
		s.silent.Add(1)
	case DropNotAllowed:
		s.notAllowed.Add(1)
	case DropQueueFull:
		s.queueFull.Add(1)
	}
}

// Totals are the totals of the packet path of a node since it started.
type Totals struct {
	Forwarded, ForwardedBytes     uint64
	Silent, NotAllowed, QueueFull uint64
}

func (s *stats) totals() Totals {
	return Totals{
		Forwarded: s.forwardedPackets.Load(), ForwardedBytes: s.forwardedBytes.Load(),
		Silent: s.silent.Load(), NotAllowed: s.notAllowed.Load(), QueueFull: s.queueFull.Load(),
	}
}
