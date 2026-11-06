// SPDX-License-Identifier: AGPL-3.0-only

package client

import "time"

// SetReloadInterval changes how often c looks at its TLS files for a change.
func (c *Conn) SetReloadInterval(d time.Duration) {
	if c.tlsFiles != nil {
		c.tlsFiles.setInterval(d)
	}
}
