// SPDX-License-Identifier: AGPL-3.0-only

// Package kind says what the server delegates to plugins and how far (ADR-0146): the kinds, their defaults and limits,
// and the rules of a destination. It imports the standard library only, so that the configuration reads it in a build
// whose tag no_plugins left the client out.
package kind

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// The protocols a kind may speak (ADR-0146 §3).
const (
	// ProtocolJSON is the Connect unary protocol with application/json, which a plugin of net/http and JSON can serve.
	ProtocolJSON = "json"
	// ProtocolProto is the Connect protocol with application/proto.
	ProtocolProto = "proto"
	// ProtocolGRPC is gRPC, always over HTTP/2.
	ProtocolGRPC = "grpc"
)

// Protocols lists the values of a protocol.
func Protocols() []string { return []string{ProtocolJSON, ProtocolProto, ProtocolGRPC} }

// MaxTimeout bounds the deadline of a call: a job's context ends with its lease, five minutes, so a call as long as that
// would be cut by the lease rather than by its own deadline (ADR-0146 §12).
const MaxTimeout = 5 * time.Minute

// DefaultConcurrency is how many calls of a kind run at once unless the kind says otherwise, and how many connections a
// client opens (ADR-0146 §7, §12).
const DefaultConcurrency = 8

const (
	kib = 1 << 10
	mib = 1 << 20
)

// The names of the kinds.
const (
	Probe         = "probe"
	Translator    = "translator"
	Billing       = "billing"
	Mailer        = "mailer"
	Media         = "media"
	KeyManager    = "key_manager"
	InboundStatus = "inbound.status"
	InboundFollow = "inbound.follow"
	SignupCheck   = "signup.check"
	MediaCheck    = "media.check"
	LocalStatus   = "local.status"
)

// Kind is one thing the server can delegate, and the limits of its calls.
type Kind struct {
	// Name is what `my plugin test --kind` takes.
	Name string
	// Protocol is the default protocol.
	Protocol string
	// Streaming is a kind whose RPCs stream both ways, which needs HTTP/2 and so refuses ProtocolJSON (ADR-0146 §13).
	Streaming bool
	// PlainTCP allows an http:// destination, with a warning. A kind that carries secrets that spread further when
	// they leak, or the text of every post, refuses it, on loopback too (ADR-0146 §6).
	PlainTCP bool
	// Timeout is the default deadline of one call.
	Timeout time.Duration
	// MaxSend and MaxReceive bound one message, in bytes. The library's own default is never relied on (ADR-0146 §12).
	MaxSend, MaxReceive int
	// Concurrency is how many calls run at once.
	Concurrency int
}

// The kinds that have a destination in the configuration, in documentation order: ADR-0146 §2, §6, §12, and the waits
// of ADR-0147 §2.
var delegated = []Kind{
	{Name: Translator, Protocol: ProtocolJSON, PlainTCP: true, Timeout: 10 * time.Second, MaxSend: mib, MaxReceive: mib, Concurrency: DefaultConcurrency},
	{Name: Billing, Protocol: ProtocolJSON, PlainTCP: true, Timeout: 2 * time.Minute, MaxSend: 64 * kib, MaxReceive: 4 * mib, Concurrency: DefaultConcurrency},
	{Name: Mailer, Protocol: ProtocolJSON, Timeout: 30 * time.Second, MaxSend: 2 * mib, MaxReceive: 64 * kib, Concurrency: DefaultConcurrency},
	{Name: Media, Protocol: ProtocolProto, Streaming: true, PlainTCP: true, Timeout: 4 * time.Minute, MaxSend: 2 * mib, MaxReceive: 2 * mib, Concurrency: 2},
	{Name: KeyManager, Protocol: ProtocolJSON, Timeout: 5 * time.Second, MaxSend: 128 * kib, MaxReceive: 128 * kib, Concurrency: DefaultConcurrency},
	{Name: InboundStatus, Protocol: ProtocolJSON, Timeout: 3 * time.Second, MaxSend: mib, MaxReceive: 64 * kib, Concurrency: DefaultConcurrency},
	{Name: InboundFollow, Protocol: ProtocolJSON, PlainTCP: true, Timeout: 3 * time.Second, MaxSend: mib, MaxReceive: 64 * kib, Concurrency: DefaultConcurrency},
	{Name: SignupCheck, Protocol: ProtocolJSON, PlainTCP: true, Timeout: 2 * time.Second, MaxSend: mib, MaxReceive: 64 * kib, Concurrency: DefaultConcurrency},
	{Name: MediaCheck, Protocol: ProtocolJSON, PlainTCP: true, Timeout: 10 * time.Second, MaxSend: mib, MaxReceive: 64 * kib, Concurrency: DefaultConcurrency},
	{Name: LocalStatus, Protocol: ProtocolJSON, Timeout: time.Second, MaxSend: mib, MaxReceive: 64 * kib, Concurrency: DefaultConcurrency},
}

// probe has no destination in the configuration: it delegates nothing, and only `my plugin test` calls it.
var probe = Kind{Name: Probe, Protocol: ProtocolJSON, PlainTCP: true, Timeout: 10 * time.Second, MaxSend: mib, MaxReceive: mib, Concurrency: DefaultConcurrency}

// Delegated returns the kinds that have a destination in the configuration, in documentation order.
func Delegated() []Kind { return slices.Clone(delegated) }

// ByName returns the kind of name, the probe included.
func ByName(name string) (Kind, bool) {
	if name == Probe {
		return probe, true
	}
	for _, k := range delegated {
		if k.Name == name {
			return k, true
		}
	}
	return Kind{}, false
}

// Names returns the names of every kind, the probe included.
func Names() []string {
	names := []string{Probe}
	for _, k := range delegated {
		names = append(names, k.Name)
	}
	return names
}

// Key returns the configuration key of the destination's suffix, such as plugin_key_manager_address.
func (k Kind) Key(suffix string) string {
	return "plugin_" + strings.ReplaceAll(k.Name, ".", "_") + "_" + suffix
}

// The suffixes of the keys of a destination.
const (
	SuffixAddress     = "address"
	SuffixProtocol    = "protocol"
	SuffixToken       = "token"
	SuffixTLSCAFile   = "tls_ca_file"
	SuffixTLSCertFile = "tls_cert_file"
	SuffixTLSKeyFile  = "tls_key_file"
	SuffixTimeout     = "timeout"
)

// Destination is where a kind's plugin listens and how to reach it. Only the configuration file and the environment set
// it: nothing in the database, the admin UI, or the client API can (ADR-0146 §5).
type Destination struct {
	// Address is unix:///path, https://host[:port], or http://host[:port]; empty delegates nothing.
	Address string
	// Protocol is one of Protocols; empty takes the kind's default.
	Protocol string
	// Token is the bearer token.
	Token string
	// TLSCAFile verifies the server in place of the system's roots; TLSCertFile and TLSKeyFile are the client's mTLS
	// certificate.
	TLSCAFile, TLSCertFile, TLSKeyFile string
	// Timeout is the deadline of one call; zero takes the kind's default.
	Timeout time.Duration
}

// Enabled reports whether the destination names a plugin.
func (d Destination) Enabled() bool { return d.Address != "" }

// Problem is what is wrong with one setting of a destination.
type Problem struct {
	// Suffix names the setting, one of the Suffix constants.
	Suffix  string
	Message string
}

// Check returns what is wrong with d for this kind. It resolves no name and reads no file (ADR-0146 §5).
func (k Kind) Check(d Destination) []Problem {
	var out []Problem
	problem := func(suffix, format string, args ...any) {
		out = append(out, Problem{Suffix: suffix, Message: fmt.Sprintf(format, args...)})
	}
	addr, err := ParseAddress(d.Address)
	if err != nil {
		problem(SuffixAddress, "%v", err)
		return out
	}
	if addr.Scheme == SchemeHTTP && !k.PlainTCP {
		problem(SuffixAddress, "must not be http://, on loopback neither: the %s plugin carries data that must not cross plain TCP; use unix:// or https://", k.Name)
	}
	if d.Protocol != "" && !slices.Contains(Protocols(), d.Protocol) {
		problem(SuffixProtocol, "must be one of %s", strings.Join(Protocols(), ", "))
	}
	if k.Streaming && d.Protocol == ProtocolJSON {
		problem(SuffixProtocol, "must be proto or grpc: the %s plugin streams both ways, which needs HTTP/2", k.Name)
	}
	if addr.Scheme == SchemeHTTP && d.Token != "" {
		problem(SuffixToken, "must not be sent over http://, where anyone on the network can read it; use unix:// or https://")
	}
	if addr.Scheme != SchemeHTTPS {
		if d.TLSCAFile != "" {
			problem(SuffixTLSCAFile, "needs an https:// address")
		}
		if d.TLSCertFile != "" {
			problem(SuffixTLSCertFile, "needs an https:// address")
		}
		if d.TLSKeyFile != "" {
			problem(SuffixTLSKeyFile, "needs an https:// address")
		}
	} else {
		if d.TLSCertFile != "" && d.TLSKeyFile == "" {
			problem(SuffixTLSKeyFile, "is required with the certificate file")
		}
		if d.TLSKeyFile != "" && d.TLSCertFile == "" {
			problem(SuffixTLSCertFile, "is required with the key file")
		}
	}
	if d.Timeout < 0 || d.Timeout >= MaxTimeout {
		problem(SuffixTimeout, "must be more than 0 and less than %s, the lease of the job that makes the call", MaxTimeout)
	}
	return out
}

// Warnings returns what an operator should know about a destination that Check accepts: who can read and call a plugin
// that nothing authenticates (ADR-0146 §6).
func (k Kind) Warnings(d Destination) []Problem {
	addr, err := ParseAddress(d.Address)
	if err != nil {
		return nil
	}
	switch addr.Scheme {
	case SchemeHTTP:
		return []Problem{{Suffix: SuffixAddress, Message: fmt.Sprintf(
			"is plain TCP: the %s plugin at %s is neither encrypted nor authenticated, so keep the network between the server and the plugin closed",
			k.Name, d.Address)}}
	case SchemeHTTPS:
		if d.Token == "" && d.TLSCertFile == "" {
			return []Problem{{Suffix: SuffixAddress, Message: fmt.Sprintf(
				"has no bearer token and no client certificate: the %s plugin at %s cannot tell the server from anyone else who reaches it",
				k.Name, d.Address)}}
		}
	}
	return nil
}
