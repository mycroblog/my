// SPDX-License-Identifier: AGPL-3.0-only

package sysmetrics

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// LabelCPUMode is the label of ProcessCPUTime: user or system.
const LabelCPUMode = "cpu.mode"

// The process metrics of ADR-0055 §10, named after the OpenTelemetry semantic conventions. The
// CPU time, the memory, and the file descriptors are read on Linux only; elsewhere only the
// uptime is reported. The role and the version are the resource of ADR-0055 §6, not metrics.
var (
	ProcessCPUTime = kernel.Instrument{
		Name: "process.cpu.time", Unit: "s", Description: "Total CPU time of the process since it started, by mode.",
		LabelKeys: []string{LabelCPUMode},
	}
	ProcessMemoryUsage = kernel.Instrument{
		Name: "process.memory.usage", Unit: "By", Description: "Resident memory of the process (RSS).",
	}
	ProcessFileDescriptors = kernel.Instrument{
		Name: "process.unix.file_descriptor.count", Unit: "{file_descriptor}", Description: "Open file descriptors of the process.",
	}
	ProcessFileDescriptorLimit = kernel.Instrument{
		Name: "my.process.file_descriptor.limit", Unit: "{file_descriptor}",
		Description: "Most file descriptors the process may open (the soft RLIMIT_NOFILE).",
	}
	ProcessUptime = kernel.Instrument{
		Name: "process.uptime", Unit: "s", Description: "Time since the process started.",
	}
)

// RegisterProcess reports the process metrics of this process, which started at start.
func RegisterProcess(m kernel.Meter, start time.Time) {
	if kernel.IsNop(m) {
		return
	}
	m.Gauge(ProcessUptime, func(_ context.Context, o kernel.GaugeObserver) error {
		o.Observe(time.Since(start).Seconds(), kernel.Labels{})
		return nil
	})
	registerOSProcess(m)
}
