// SPDX-License-Identifier: AGPL-3.0-only

package kind

import (
	"errors"
	"net/url"
	"path"
)

// The schemes of an address.
const (
	SchemeUnix  = "unix"
	SchemeHTTPS = "https"
	SchemeHTTP  = "http"
)

// Address is a parsed destination address.
type Address struct {
	// Scheme is one of the Scheme constants.
	Scheme string
	// SocketPath is the path of the Unix socket of SchemeUnix.
	SocketPath string
	// BaseURL is what the RPC paths follow: the address itself, or a fixed name for a socket, which has no host.
	BaseURL string
}

// ParseAddress checks the shape of an address, without resolving a name: the name is resolved per connection
// (ADR-0146 §5).
func ParseAddress(s string) (Address, error) {
	const shape = "must be unix:///path/to.sock, https://host[:port], or http://host[:port]"
	u, err := url.Parse(s)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Address{}, errors.New(shape + ", without credentials, a query, or a fragment")
	}
	switch u.Scheme {
	case SchemeUnix:
		if u.Host != "" || !path.IsAbs(u.Path) || path.Clean(u.Path) != u.Path || u.Path == "/" {
			return Address{}, errors.New(shape + ": a socket takes an absolute path after unix://")
		}
		return Address{Scheme: SchemeUnix, SocketPath: u.Path, BaseURL: "http://localhost"}, nil
	case SchemeHTTPS, SchemeHTTP:
		if u.Hostname() == "" {
			return Address{}, errors.New(shape + ": the host is missing")
		}
		return Address{Scheme: u.Scheme, BaseURL: s}, nil
	}
	return Address{}, errors.New(shape)
}
