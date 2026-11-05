// SPDX-License-Identifier: AGPL-3.0-only

package search

// Service is the search module. Its index is nil with search = none (ADR-0149 §2).
type Service struct {
	index Index
}

// NewService returns the module over index, nil with search = none.
func NewService(index Index) *Service {
	return &Service{index: index}
}

// Enabled reports whether the server has a backend, which is false with search = none.
func (s *Service) Enabled() bool {
	return s.index != nil
}

// Visibilities returns the visibilities the full-text search covers, none with search = none.
// With a backend it is public, the default of search_visibilities (ADR-0149 §5).
func (s *Service) Visibilities() []Visibility {
	if !s.Enabled() {
		return nil
	}
	return []Visibility{VisibilityPublic}
}
