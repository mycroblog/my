// SPDX-License-Identifier: AGPL-3.0-only

package db_test

import (
	"io"
	"log/slog"
)

// discardLogger drops the logs of the signals that the wake contract makes fail on purpose.
func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
