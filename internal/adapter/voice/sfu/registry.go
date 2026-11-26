// SPDX-License-Identifier: AGPL-3.0-only

package sfu

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The leases of the registry (ADR-0101 §14): a node renews its own lease, its reservations, and
// the assignments of its rooms every LeaseRenewal, and each expires LeaseTTL after its last
// renewal, so that the rooms of a node that stopped renewing are placed again.
const (
	LeaseRenewal = 5 * time.Second
	LeaseTTL     = 15 * time.Second
	// TicketTTL is how long a ticket connects (ADR-0101 §11).
	TicketTTL = 30 * time.Second
	// reservationGrace keeps a reservation whose room is not assigned to the node yet: a
	// placement reserves first and assigns after.
	reservationGrace = 30 * time.Second
	// casAttempts bounds the retries of a compare-and-swap that other processes keep winning.
	casAttempts = 16
)

// The namespaces of the registry in the KeyValueStore.
const (
	// NamespaceNodes holds, under the key NodesKey, the IDs of the nodes that registered, a JSON
	// array; an ID whose lease expired is pruned when the nodes are read.
	NamespaceNodes = "voice.sfu.nodes"
	NodesKey       = "all"
	// NamespaceNode holds the lease of a node (NodeLease) by node ID.
	NamespaceNode = "voice.sfu.node"
	// NamespaceReserved holds the reservations of a node by node ID: a JSON object of the
	// reserved maximum of each room placed on it, by room ID. The sum of its maximums never
	// exceeds the node's capacity: every change is a compare-and-swap that checks it.
	NamespaceReserved = "voice.sfu.reserved"
	// NamespaceRoom holds the assignment of a room to a node (Assignment) by room ID.
	NamespaceRoom = "voice.sfu.room"
	// NamespaceTicket holds the tickets (Ticket) by their token.
	NamespaceTicket = "voice.sfu.ticket"
)

// The errors of the registry.
var (
	// ErrNoCapacity reports that no media node has room for the maximum of a room (ADR-0101
	// §14): the opening or the join answers 503.
	ErrNoCapacity = errors.New("sfu: no media node has room for the room")
	// ErrInvalidTicket reports a ticket that expired or was used.
	ErrInvalidTicket = errors.New("sfu: the ticket expired or was used")
	// ErrOtherNode reports a ticket for another node.
	ErrOtherNode = errors.New("sfu: the ticket is for another node")
	// errContended reports a compare-and-swap that kept losing to other processes.
	errContended = errors.New("sfu: the registry kept changing meanwhile")
)

// NodeLease is the lease of a media node: what a placement reads of it.
type NodeLease struct {
	ID string `json:"id"`
	// URL is the wss: URL that the clients connect to (voice_sfu_public_url).
	URL string `json:"url"`
	// Capacity is how many participants the node holds (voice_sfu_node_capacity).
	Capacity int `json:"capacity"`
	// Reserved is the sum of the maximums of the rooms placed on the node, and Connected the
	// sessions connected to it, when it renewed.
	Reserved  int `json:"reserved"`
	Connected int `json:"connected"`
	// Draining reports a node that is stopping: it takes no new room.
	Draining bool `json:"draining,omitempty"`
}

// Assignment is the node of a room.
type Assignment struct {
	Node string `json:"node"`
	// Max is the maximum that the room reserved on the node.
	Max int `json:"max"`
	// Sessions are the sessions connected to the room on the node, when it renewed.
	Sessions []ConnectedSession `json:"sessions,omitempty"`
}

// ConnectedSession is a session connected to a node.
type ConnectedSession struct {
	ID        string `json:"id"`
	AccountID string `json:"account"`
}

// Ticket is what a ticket connects: a session of a room, on one node (ADR-0101 §11).
type Ticket struct {
	Room    string `json:"room"`
	Session string `json:"session"`
	Account string `json:"account"`
	Node    string `json:"node"`
}

// reservation is the maximum that a room reserved on a node, and when.
type reservation struct {
	Max int       `json:"max"`
	At  time.Time `json:"at"`
}

// Registry is the registry of the media nodes in the KeyValueStore that the web processes and
// the nodes share (ADR-0101 §14): the leases of the nodes, their reservations, the assignments
// of the rooms, and the tickets. Every change that other processes may race is a
// compare-and-swap, so that a single process (role = all, KeyValueStore = db) and many need the
// same nothing.
type Registry struct {
	kv    kernel.KeyValueStore
	clock kernel.Clock
}

// NewRegistry returns the Registry on kv.
func NewRegistry(kv kernel.KeyValueStore, clock kernel.Clock) *Registry {
	return &Registry{kv: kv, clock: clock}
}

// get returns the raw value of a key, nil when there is none.
func (r *Registry) get(ctx context.Context, namespace, key string) ([]byte, error) {
	b, err := r.kv.Get(ctx, namespace, key)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return nil, nil
	}
	return b, err
}

// update changes the value of a key by a compare-and-swap with what it read: change returns the
// new value, nil to delete it, or ok false to leave it.
func (r *Registry) update(ctx context.Context, namespace, key string, ttl time.Duration, change func(old []byte) (next []byte, ok bool, err error)) error {
	for range casAttempts {
		old, err := r.get(ctx, namespace, key)
		if err != nil {
			return err
		}
		next, ok, err := change(old)
		if err != nil || !ok {
			return err
		}
		if old == nil && next == nil {
			return nil
		}
		swapped, err := r.kv.CompareAndSwap(ctx, namespace, key, old, next, ttl)
		if err != nil || swapped {
			return err
		}
	}
	return errContended
}

// Lease returns the lease of a node, found false when it expired.
func (r *Registry) Lease(ctx context.Context, id string) (NodeLease, bool, error) {
	b, err := r.get(ctx, NamespaceNode, id)
	if err != nil || b == nil {
		return NodeLease{}, false, err
	}
	var l NodeLease
	if err := json.Unmarshal(b, &l); err != nil {
		return NodeLease{}, false, fmt.Errorf("sfu: the lease of node %s: %w", id, err)
	}
	return l, true, nil
}

// Nodes returns the leases of the live nodes, by ID. The IDs of the nodes whose lease expired
// leave the list.
func (r *Registry) Nodes(ctx context.Context) ([]NodeLease, error) {
	b, err := r.get(ctx, NamespaceNodes, NodesKey)
	if err != nil {
		return nil, err
	}
	var ids []string
	if b != nil {
		if err := json.Unmarshal(b, &ids); err != nil {
			return nil, fmt.Errorf("sfu: the list of nodes: %w", err)
		}
	}
	var out []NodeLease
	var gone []string
	for _, id := range ids {
		l, ok, err := r.Lease(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			gone = append(gone, id)
			continue
		}
		out = append(out, l)
	}
	if len(gone) > 0 {
		// Best effort: the next read prunes what a race kept.
		next, _ := json.Marshal(slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return slices.Contains(gone, id) }))
		_, _ = r.kv.CompareAndSwap(ctx, NamespaceNodes, NodesKey, b, next, 0)
	}
	slices.SortFunc(out, func(a, b NodeLease) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// Register writes the lease of a node and lists it among the nodes.
func (r *Registry) Register(ctx context.Context, l NodeLease) error {
	if err := r.writeLease(ctx, l); err != nil {
		return err
	}
	return r.update(ctx, NamespaceNodes, NodesKey, 0, func(old []byte) ([]byte, bool, error) {
		var ids []string
		if old != nil {
			if err := json.Unmarshal(old, &ids); err != nil {
				return nil, false, err
			}
		}
		if slices.Contains(ids, l.ID) {
			return nil, false, nil
		}
		next, err := json.Marshal(append(ids, l.ID))
		return next, true, err
	})
}

func (r *Registry) writeLease(ctx context.Context, l NodeLease) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return r.kv.Set(ctx, NamespaceNode, l.ID, b, LeaseTTL)
}

// Deregister removes the lease of a node that stopped, and its reservations; the assignments
// of its rooms expire.
func (r *Registry) Deregister(ctx context.Context, id string) error {
	return errors.Join(r.kv.Delete(ctx, NamespaceNode, id), r.kv.Delete(ctx, NamespaceReserved, id))
}

// HeldRoom is a room assigned to a node, with the maximum it reserved there.
type HeldRoom struct {
	ID  string
	Max int
}

// Renew renews the lease of a node, its reservations, and the assignments of its rooms with the
// sessions connected to each (sessions, by room ID), and returns the rooms assigned to it. A
// reservation whose room is assigned to another node, or to none for longer than a placement
// takes, is dropped. l.Reserved is set from the reservations.
func (r *Registry) Renew(ctx context.Context, l NodeLease, sessions map[string][]ConnectedSession) ([]HeldRoom, error) {
	var held []HeldRoom
	now := r.clock.Now()
	err := r.update(ctx, NamespaceReserved, l.ID, LeaseTTL, func(old []byte) ([]byte, bool, error) {
		held = held[:0]
		rs, err := decodeReservations(old)
		if err != nil {
			return nil, false, err
		}
		for room, res := range rs {
			a, ok, err := r.Assignment(ctx, room)
			if err != nil {
				return nil, false, err
			}
			switch {
			case ok && a.Node == l.ID:
				held = append(held, HeldRoom{ID: room, Max: res.Max})
			case !ok && now.Sub(res.At) < reservationGrace:
			default:
				delete(rs, room)
			}
		}
		if len(rs) == 0 {
			return nil, true, nil
		}
		next, err := json.Marshal(rs)
		return next, true, err
	})
	if err != nil {
		return nil, err
	}
	l.Reserved = 0
	for _, h := range held {
		l.Reserved += h.Max
		if err := r.renewAssignment(ctx, h.ID, l.ID, sessions[h.ID]); err != nil {
			return nil, err
		}
	}
	if err := r.Register(ctx, l); err != nil {
		return nil, err
	}
	slices.SortFunc(held, func(a, b HeldRoom) int { return cmp.Compare(a.ID, b.ID) })
	return held, nil
}

// renewAssignment renews the assignment of a room to node while it lasts.
func (r *Registry) renewAssignment(ctx context.Context, room, node string, sessions []ConnectedSession) error {
	return r.update(ctx, NamespaceRoom, room, LeaseTTL, func(old []byte) ([]byte, bool, error) {
		a, ok, err := decodeAssignment(old)
		if err != nil || !ok || a.Node != node {
			return nil, false, err
		}
		a.Sessions = sessions
		next, err := json.Marshal(a)
		return next, true, err
	})
}

// Assignment returns the assignment of a room, found false when it has none.
func (r *Registry) Assignment(ctx context.Context, room string) (Assignment, bool, error) {
	b, err := r.get(ctx, NamespaceRoom, room)
	if err != nil {
		return Assignment{}, false, err
	}
	return decodeAssignment(b)
}

func decodeAssignment(b []byte) (Assignment, bool, error) {
	if b == nil {
		return Assignment{}, false, nil
	}
	var a Assignment
	if err := json.Unmarshal(b, &a); err != nil {
		return Assignment{}, false, fmt.Errorf("sfu: an assignment: %w", err)
	}
	return a, true, nil
}

func decodeReservations(b []byte) (map[string]reservation, error) {
	rs := map[string]reservation{}
	if b == nil {
		return rs, nil
	}
	if err := json.Unmarshal(b, &rs); err != nil {
		return nil, fmt.Errorf("sfu: the reservations of a node: %w", err)
	}
	return rs, nil
}

// reserved returns the sum of the maximums reserved on a node.
func (r *Registry) reserved(ctx context.Context, node string) (int, error) {
	b, err := r.get(ctx, NamespaceReserved, node)
	if err != nil {
		return 0, err
	}
	rs, err := decodeReservations(b)
	if err != nil {
		return 0, err
	}
	sum := 0
	for _, res := range rs {
		sum += res.Max
	}
	return sum, nil
}

// reserve reserves maximum for a room on a node whose capacity is capacity, and reports whether it
// fit. A room reserved there already fits.
func (r *Registry) reserve(ctx context.Context, node, room string, maximum, capacity int) (bool, error) {
	fit := false
	err := r.update(ctx, NamespaceReserved, node, LeaseTTL, func(old []byte) ([]byte, bool, error) {
		rs, err := decodeReservations(old)
		if err != nil {
			return nil, false, err
		}
		if _, ok := rs[room]; ok {
			fit = true
			return nil, false, nil
		}
		sum := 0
		for _, res := range rs {
			sum += res.Max
		}
		if fit = sum+maximum <= capacity; !fit {
			return nil, false, nil
		}
		rs[room] = reservation{Max: maximum, At: r.clock.Now()}
		next, err := json.Marshal(rs)
		return next, true, err
	})
	return fit, err
}

// unreserve drops the reservation of a room on a node.
func (r *Registry) unreserve(ctx context.Context, node, room string) error {
	return r.update(ctx, NamespaceReserved, node, LeaseTTL, func(old []byte) ([]byte, bool, error) {
		rs, err := decodeReservations(old)
		if err != nil {
			return nil, false, err
		}
		if _, ok := rs[room]; !ok {
			return nil, false, nil
		}
		delete(rs, room)
		if len(rs) == 0 {
			return nil, true, nil
		}
		next, err := json.Marshal(rs)
		return next, true, err
	})
}

// Place returns the node of a room, placing it when it has none, when its node is gone or
// draining, or when its node is avoid (a draining node that moves its rooms away, Move): the
// room goes to the live node, not draining and not avoid, with the smallest sum of reserved
// maximums that still has room for maximum (ADR-0101 §14). When no other node has room, a room on a
// live node that is draining (or avoid) stays there; one on no live node fails with
// ErrNoCapacity. Concurrent placements of the room agree on one node, and no reservation ever
// takes a node past its capacity.
func (r *Registry) Place(ctx context.Context, room string, maximum int, avoid string) (NodeLease, error) {
	for range casAttempts {
		raw, err := r.get(ctx, NamespaceRoom, room)
		if err != nil {
			return NodeLease{}, err
		}
		cur, assigned, err := decodeAssignment(raw)
		if err != nil {
			return NodeLease{}, err
		}
		var holder *NodeLease
		if assigned {
			l, live, err := r.Lease(ctx, cur.Node)
			if err != nil {
				return NodeLease{}, err
			}
			if live {
				holder = &l
				if !l.Draining && l.ID != avoid {
					return l, nil
				}
			}
		}
		nodes, err := r.candidates(ctx, avoid)
		if err != nil {
			return NodeLease{}, err
		}
		lost := false
		for _, n := range nodes {
			if assigned && n.ID == cur.Node {
				continue
			}
			fit, err := r.reserve(ctx, n.ID, room, maximum, n.Capacity)
			if err != nil {
				return NodeLease{}, err
			}
			if !fit {
				continue
			}
			next, err := json.Marshal(Assignment{Node: n.ID, Max: maximum})
			if err != nil {
				return NodeLease{}, err
			}
			swapped, err := r.kv.CompareAndSwap(ctx, NamespaceRoom, room, raw, next, LeaseTTL)
			if err != nil {
				return NodeLease{}, err
			}
			if !swapped {
				// Another placement won. The reservation is the room's on that node, whoever
				// made it: free it only when the room went elsewhere, and follow the winner.
				won, ok, err := r.Assignment(ctx, room)
				if err != nil {
					return NodeLease{}, err
				}
				if !ok || won.Node != n.ID {
					if err := r.unreserve(ctx, n.ID, room); err != nil {
						return NodeLease{}, err
					}
				}
				lost = true
				break
			}
			if assigned {
				// The node renews its reservations and drops this one anyway; dropping it now
				// frees the room's place on a draining node at once.
				_ = r.unreserve(ctx, cur.Node, room)
			}
			return n, nil
		}
		if lost {
			continue
		}
		if holder != nil {
			return *holder, nil
		}
		return NodeLease{}, ErrNoCapacity
	}
	return NodeLease{}, errContended
}

// candidates returns the live nodes that are not draining and not avoid, the one with the
// smallest sum of reserved maximums first.
func (r *Registry) candidates(ctx context.Context, avoid string) ([]NodeLease, error) {
	nodes, err := r.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	nodes = slices.DeleteFunc(nodes, func(n NodeLease) bool { return n.Draining || n.ID == avoid })
	for i := range nodes {
		if nodes[i].Reserved, err = r.reserved(ctx, nodes[i].ID); err != nil {
			return nil, err
		}
	}
	slices.SortStableFunc(nodes, func(a, b NodeLease) int { return cmp.Compare(a.Reserved, b.Reserved) })
	return nodes, nil
}

// Release removes the assignment of a room and its reservation.
func (r *Registry) Release(ctx context.Context, room string) error {
	var node string
	err := r.update(ctx, NamespaceRoom, room, LeaseTTL, func(old []byte) ([]byte, bool, error) {
		a, ok, err := decodeAssignment(old)
		if err != nil || !ok {
			return nil, false, err
		}
		node = a.Node
		return nil, true, nil
	})
	if err != nil || node == "" {
		return err
	}
	return r.unreserve(ctx, node, room)
}

// Issue writes a ticket and returns its token, which connects once within TicketTTL.
func (r *Registry) Issue(ctx context.Context, t Ticket) (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	token := rand.Text()
	if err := r.kv.Set(ctx, NamespaceTicket, token, b, TicketTTL); err != nil {
		return "", err
	}
	return token, nil
}

// Consume uses the ticket of token on node: it fails with ErrInvalidTicket for a ticket that
// expired or was used, and ErrOtherNode for one of another node, which is used up as well.
func (r *Registry) Consume(ctx context.Context, token, node string) (Ticket, error) {
	if token == "" {
		return Ticket{}, ErrInvalidTicket
	}
	b, err := r.get(ctx, NamespaceTicket, token)
	if err != nil {
		return Ticket{}, err
	}
	if b == nil {
		return Ticket{}, ErrInvalidTicket
	}
	used, err := r.kv.CompareAndSwap(ctx, NamespaceTicket, token, b, nil, 0)
	if err != nil {
		return Ticket{}, err
	}
	if !used {
		return Ticket{}, ErrInvalidTicket
	}
	var t Ticket
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&t); err != nil {
		return Ticket{}, ErrInvalidTicket
	}
	if t.Node != node {
		return Ticket{}, ErrOtherNode
	}
	return t, nil
}
