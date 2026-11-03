// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"fmt"
	"time"
)

// Domains are the host and account_domain that a database belongs to (ADR-0018). They are recorded
// once, because other servers store ids under Host and handles under AccountDomain.
type Domains struct {
	Host          string
	AccountDomain string
	RecordedAt    time.Time
}

// Same reports whether both domains equal those of o.
func (d Domains) Same(o Domains) bool {
	return d.Host == o.Host && d.AccountDomain == o.AccountDomain
}

// DomainMismatchError reports a configuration whose domains differ from those of the database.
type DomainMismatchError struct {
	Recorded   Domains
	Configured Domains
}

func (e *DomainMismatchError) Error() string {
	return fmt.Sprintf("the database belongs to host %s and account_domain %s, but the configuration sets host %s and account_domain %s: "+
		"other servers store ids and handles made from these values, so they cannot change; "+
		"set the recorded values in the configuration, or point database_url at the database of the configured ones",
		e.Recorded.Host, e.Recorded.AccountDomain, e.Configured.Host, e.Configured.AccountDomain)
}
