// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The label keys of the TLS metrics (ADR-0163 §11). The hosts are those of the configuration,
// a set fixed at start.
const (
	LabelTLSHost   = "host"
	LabelTLSResult = "result"
)

// The values of LabelTLSResult.
const (
	TLSOrderIssued = "issued"
	TLSOrderFailed = "failed"
)

// The metrics of the certificates that the server serves itself (ADR-0163 §11).
var (
	// TLSCertificateExpiry is reported by each process from the certificates in its memory, so
	// that a process left with an old certificate shows; internal/platform registers it, for tls
	// = files too.
	TLSCertificateExpiry = kernel.Instrument{
		Name: "my.tls.certificate.expiry", Unit: "s",
		Description: "Time until the certificate that this process serves for each host expires.",
		LabelKeys:   []string{LabelTLSHost},
	}
	TLSOrderAttempts = kernel.Instrument{
		Name: "my.tls.order.attempts", Unit: "{order}",
		Description: "Orders of TLS certificates from the ACME certificate authority, by result.",
		LabelKeys:   []string{LabelTLSHost, LabelTLSResult},
	}
	TLSOrderConsecutiveFailures = kernel.Instrument{
		Name: "my.tls.order.consecutive_failures", Unit: "{order}",
		Description: "Orders of each host since its current certificate that obtained no certificate.",
		LabelKeys:   []string{LabelTLSHost},
	}
	// TLSRounds counts the rounds of TLSManager.Maintain that ended, each over every host: a
	// manager whose count stops growing has stopped.
	TLSRounds = kernel.Instrument{
		Name: "my.tls.rounds", Unit: "{round}",
		Description: "Rounds of the ACME certificate manager over its hosts that ended.",
	}
)

// registerMetrics declares the metrics of the orders on meter.
func (m *TLSManager) registerMetrics(meter kernel.Meter) {
	m.attempts = meter.Counter(TLSOrderAttempts)
	m.rounds = meter.Counter(TLSRounds)
	meter.Gauge(TLSOrderConsecutiveFailures, func(_ context.Context, o kernel.GaugeObserver) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, h := range m.opts.Hosts {
			if n, ok := m.failures[h]; ok {
				o.Observe(float64(n), kernel.NewLabels(LabelTLSHost, h))
			}
		}
		return nil
	})
}

// countOrder records the end of an order of host.
func (m *TLSManager) countOrder(ctx context.Context, host string, issued bool) {
	result := TLSOrderFailed
	m.mu.Lock()
	if issued {
		result = TLSOrderIssued
		m.failures[host] = 0
	} else {
		m.failures[host]++
	}
	m.mu.Unlock()
	m.attempts.Add(ctx, 1, kernel.NewLabels(LabelTLSHost, host, LabelTLSResult, result))
}
