// SPDX-License-Identifier: MIT

package mybot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultStatePath is the file of the FileStore that a Bot uses when Config.Store is nil.
const DefaultStatePath = "./mybot-state.json"

// Store keeps what a bot must remember across restarts (ADR-0094 §6): the notifications it processed, the
// position of the catch-up, the last runs of the scheduled jobs, and the rotated refresh token. The bot calls it
// from several goroutines. MemoryStore and FileStore are the implementations of the library; implement Store to
// keep the state elsewhere, such as in a database. One Store serves one bot process: it needs no locking between
// processes.
type Store interface {
	// IsProcessed reports whether the notification id was recorded by MarkProcessed (and not forgotten since).
	IsProcessed(ctx context.Context, id string) (bool, error)
	// MarkProcessed records that the notification id was processed at at.
	MarkProcessed(ctx context.Context, id string, at time.Time) error
	// ForgetProcessed drops the records of the notifications processed before before.
	ForgetProcessed(ctx context.Context, before time.Time) error
	// Position is the ID of the newest notification up to which every notification was processed, "" when
	// there is none yet. The catch-up reads the notifications back to it.
	Position(ctx context.Context) (string, error)
	// SetPosition sets the Position.
	SetPosition(ctx context.Context, id string) error
	// LastRun is when the scheduled job name last ran, the zero time when it never did.
	LastRun(ctx context.Context, name string) (time.Time, error)
	// SetLastRun records when the scheduled job name ran.
	SetLastRun(ctx context.Context, name string, at time.Time) error
	// RefreshToken is the refresh token saved by SetRefreshToken, "" when there is none.
	RefreshToken(ctx context.Context) (string, error)
	// SetRefreshToken saves the refresh token that a renewal returned. The previous one no longer works.
	SetRefreshToken(ctx context.Context, token string) error
}

// storeState is the state of a MemoryStore and the JSON document of a FileStore.
type storeState struct {
	Processed    map[string]time.Time `json:"processed"`
	Position     string               `json:"position,omitempty"`
	LastRuns     map[string]time.Time `json:"last_runs,omitempty"`
	RefreshToken string               `json:"refresh_token,omitempty"`
}

// stateStore is a Store in memory that calls save after every change.
type stateStore struct {
	mu   sync.Mutex
	s    storeState
	save func(*storeState) error
}

func newStateStore(s storeState, save func(*storeState) error) *stateStore {
	if s.Processed == nil {
		s.Processed = map[string]time.Time{}
	}
	if s.LastRuns == nil {
		s.LastRuns = map[string]time.Time{}
	}
	return &stateStore{s: s, save: save}
}

// change applies f to the state and saves it; s.mu is taken.
func (st *stateStore) change(f func(*storeState)) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	f(&st.s)
	if st.save == nil {
		return nil
	}
	return st.save(&st.s)
}

func (st *stateStore) IsProcessed(_ context.Context, id string) (bool, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	_, ok := st.s.Processed[id]
	return ok, nil
}

func (st *stateStore) MarkProcessed(_ context.Context, id string, at time.Time) error {
	return st.change(func(s *storeState) { s.Processed[id] = at.UTC() })
}

func (st *stateStore) ForgetProcessed(_ context.Context, before time.Time) error {
	st.mu.Lock()
	stale := false
	for _, at := range st.s.Processed {
		if at.Before(before) {
			stale = true
			break
		}
	}
	st.mu.Unlock()
	if !stale {
		return nil
	}
	return st.change(func(s *storeState) {
		for id, at := range s.Processed {
			if at.Before(before) {
				delete(s.Processed, id)
			}
		}
	})
}

func (st *stateStore) Position(context.Context) (string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s.Position, nil
}

func (st *stateStore) SetPosition(_ context.Context, id string) error {
	return st.change(func(s *storeState) { s.Position = id })
}

func (st *stateStore) LastRun(_ context.Context, name string) (time.Time, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s.LastRuns[name], nil
}

func (st *stateStore) SetLastRun(_ context.Context, name string, at time.Time) error {
	return st.change(func(s *storeState) { s.LastRuns[name] = at.UTC() })
}

func (st *stateStore) RefreshToken(context.Context) (string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s.RefreshToken, nil
}

func (st *stateStore) SetRefreshToken(_ context.Context, token string) error {
	return st.change(func(s *storeState) { s.RefreshToken = token })
}

// MemoryStore is a Store in memory, for tests. Everything is lost when the process ends, the rotated refresh
// token too: the next start uses the refresh token of the Config again, which the server has seen used and so
// revokes. A bot that runs for real, all the time or started again and again, uses a FileStore or a Store of its
// own.
type MemoryStore struct{ *stateStore }

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{newStateStore(storeState{}, nil)}
}

// FileStore is a Store in one JSON file, the default Store of a Bot. Every change rewrites the file: the new
// content goes to a temporary file in the same directory, which then replaces the file, so that a crash never
// leaves half of it. The file holds the refresh token: it is created readable by its owner only.
type FileStore struct {
	*stateStore
	path string
}

// NewFileStore returns the FileStore of the file at path, reading what it holds. A missing file is an empty
// state, created on the first change.
func NewFileStore(path string) (*FileStore, error) {
	path = filepath.Clean(path)
	var s storeState
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("mybot: read the state file: %w", err)
	default:
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("mybot: read the state file %s: %w", path, err)
		}
	}
	f := &FileStore{path: path}
	f.stateStore = newStateStore(s, f.write)
	return f, nil
}

// Path is the file of the store.
func (f *FileStore) Path() string { return f.path }

// write replaces the file with s.
func (f *FileStore) write(s *storeState) error {
	b, err := json.MarshalIndent(s, "", "  ") //nolint:gosec // G117: saving the refresh token is the purpose of the store (ADR-0094 §7)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), "."+filepath.Base(f.path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("mybot: write the state file: %w", err)
	}
	done := false
	defer func() {
		if !done {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("mybot: write the state file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("mybot: write the state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("mybot: write the state file: %w", err)
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		_ = os.Remove(tmp.Name())
		done = true
		return fmt.Errorf("mybot: write the state file: %w", err)
	}
	done = true
	return nil
}
