# SSRF-guarded HTTP client

- Adding or removing a blocked address range: [IANA IPv4 Special-Purpose Address Registry](https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml), search for: `169.254.0.0/16`, `100.64.0.0/10`, `192.0.0.0/24`, `198.18.0.0/15`
- Adding or removing a blocked IPv6 range: [IANA IPv6 Special-Purpose Address Registry](https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry.xhtml), search for: `64:ff9b::/96`, `2002::/16`, `fc00::/7`, `::ffff:0:0/96`
- Reading an IPv4 address embedded in an IPv6 address (NAT64): [RFC 6052](https://www.rfc-editor.org/rfc/rfc6052), search for: `64:ff9b::/96`, Well-Known Prefix
- Reading an IPv4-mapped IPv6 address: [RFC 4291](https://www.rfc-editor.org/rfc/rfc4291), search for: IPv4-Mapped IPv6 Address, `::ffff`
