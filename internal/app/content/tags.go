// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The hashtags by name, for the modules that keep something about a hashtag that no status may
// have used yet: the followed hashtags of the timeline module (ADR-0013 §27).

// MaxHashtagLength is the longest hashtag name, in characters without the #, that ParseHashtag
// accepts.
const MaxHashtagLength = 256

// ErrInvalidHashtag reports a name that ParseHashtag rejects.
var ErrInvalidHashtag = errors.New("content: invalid hashtag")

// ParseHashtag returns the hashtag name, with or without its leading #, as a text would link it:
// letters, marks, digits, and underscores with at least one letter or mark, at most
// MaxHashtagLength characters. It reports false for any other name.
func ParseHashtag(name string) (Hashtag, bool) {
	name = strings.TrimPrefix(name, "#")
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxHashtagLength {
		return Hashtag{}, false
	}
	seg, n, ok := scanHashtag("#" + name)
	if !ok || n != len(name)+1 {
		return Hashtag{}, false
	}
	return seg.Hashtag, true
}

// EnsureTagTx returns the stored hashtag name inside tx, storing it first when no status has
// used it yet. It fails with ErrInvalidHashtag for a name that ParseHashtag rejects.
func (s *Service) EnsureTagTx(ctx context.Context, tx uow.Tx, name string) (Tag, error) {
	h, ok := ParseHashtag(name)
	if !ok {
		return Tag{}, ErrInvalidHashtag
	}
	return s.d.Repository.EnsureTag(ctx, tx, content.Tag{
		ID: s.d.IDs.NewID(), Name: h.Name, NameNormalized: h.NameNormalized, CreatedAt: kernel.Normalize(s.d.Clock.Now()),
	})
}

// TagByName returns the stored hashtag name, matched under NormalizeTag, failing with
// ErrNotFound when it is not stored and with ErrInvalidHashtag for a name that ParseHashtag
// rejects.
func (s *Service) TagByName(ctx context.Context, name string) (Tag, error) {
	h, ok := ParseHashtag(name)
	if !ok {
		return Tag{}, ErrInvalidHashtag
	}
	return s.d.Repository.FindTagByName(ctx, h.NameNormalized)
}

// TagsByID returns the stored hashtags among ids, by ID, with one query.
func (s *Service) TagsByID(ctx context.Context, ids []string) (map[string]Tag, error) {
	if len(ids) == 0 {
		return map[string]Tag{}, nil
	}
	return s.d.Repository.FindTags(ctx, ids)
}
