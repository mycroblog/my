// SPDX-License-Identifier: AGPL-3.0-only

package identity

import "time"

// ExportOutcome is how the build of a data export ended (data_export_results.outcome).
type ExportOutcome string

// Export outcomes.
const (
	// ExportReady means the archive is in blob storage until the result's ExpiresAt.
	ExportReady ExportOutcome = "ready"
	// ExportFailed means the archive could not be built; the user may request another.
	ExportFailed ExportOutcome = "failed"
)

// ExportState is the state of a data export at a moment, derived from its result.
type ExportState string

// Export states.
const (
	// ExportStatePending is an export whose archive is being built.
	ExportStatePending ExportState = "pending"
	// ExportStateReady is an export whose archive can be downloaded.
	ExportStateReady ExportState = "ready"
	// ExportStateFailed is an export whose build failed.
	ExportStateFailed ExportState = "failed"
	// ExportStateExpired is an export whose archive is past its expiry and is being removed.
	ExportStateExpired ExportState = "expired"
)

// DataExport is a user's request for an archive of their data (FR-008).
type DataExport struct {
	ID          string
	UserID      string
	RequestedAt time.Time
	// Result is how the build ended; nil while it is being built.
	Result *DataExportResult
}

// DataExportResult is the end of the build of a data export. BlobKey and FileSize are set only
// for ExportReady.
type DataExportResult struct {
	ExportID string
	Outcome  ExportOutcome
	BlobKey  string
	FileSize int64
	// ExpiresAt is when the archive (or the record of the failure) is removed.
	ExpiresAt   time.Time
	CompletedAt time.Time
}

// State returns the state of e at now.
func (e DataExport) State(now time.Time) ExportState {
	switch {
	case e.Result == nil:
		return ExportStatePending
	case !e.Result.ExpiresAt.After(now):
		return ExportStateExpired
	case e.Result.Outcome == ExportReady:
		return ExportStateReady
	default:
		return ExportStateFailed
	}
}
