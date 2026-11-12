// SPDX-License-Identifier: AGPL-3.0-only

package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// ServerFile is the file of Dir in which the dev server records its URL while it listens (web/devtools/src/vite.ts);
// it is not a batch.
const ServerFile = "server.json"

// StartCommand starts the dev server of the catalog page.
const StartCommand = "scripts/task.sh web:catalog"

// ErrNoServer is returned when no dev server is recorded, or the recorded one does not answer.
var ErrNoServer = errors.New("no dev server runs: start it with " + StartCommand)

// Recorded returns the URL the dev server recorded in dir, or "" when it recorded none.
func Recorded(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ServerFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var s struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return "", fmt.Errorf("%s: %w", ServerFile, err)
	}
	return s.URL, nil
}

// URL returns the URL of the running dev server: the recorded one, when it answers an HTTP request (any status) with
// client. Otherwise it returns ErrNoServer.
func URL(ctx context.Context, dir string, client *http.Client) (string, error) {
	url, err := Recorded(dir)
	if err != nil {
		return "", err
	}
	if url == "" {
		return "", ErrNoServer
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ServerFile, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		// A server that stopped without removing the file (a killed process).
		return "", ErrNoServer
	}
	_ = resp.Body.Close()
	return url, nil
}
