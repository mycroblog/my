// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/content"
	domain "github.com/mycroblog/my/internal/domain/content"
)

// imagesRepo is a MediaRepository with no uploads and the remote files of files. Like PostgreSQL,
// whose media IDs are uuid columns, it refuses an empty ID.
type imagesRepo struct {
	content.MediaRepository
	files  map[string]domain.RemoteMedia
	remote [][]string
}

func (r *imagesRepo) ListReadyVariants(_ context.Context, ids []string) ([]content.FileVariant, error) {
	if slices.Contains(ids, "") {
		return nil, errors.New(`invalid input syntax for type uuid: ""`)
	}
	return nil, nil
}

func (r *imagesRepo) ListRemoteMedia(_ context.Context, ids []string) ([]domain.RemoteMedia, error) {
	r.remote = append(r.remote, ids)
	if slices.Contains(ids, "") {
		return nil, errors.New(`invalid input syntax for type uuid: ""`)
	}
	var out []domain.RemoteMedia
	for _, id := range ids {
		if m, ok := r.files[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// TestImagesWithoutAMediaID checks that a ref without a media ID, such as the header of a
// profile that has an avatar and no header, has no image and reaches no read: PostgreSQL refuses
// an empty uuid, which failed the fan-out of such an account (#2665).
func TestImagesWithoutAMediaID(t *testing.T) {
	repo := &imagesRepo{files: map[string]domain.RemoteMedia{
		remoteMP: {MediaID: remoteMP, OwnerAccountID: ownerID, RemoteURL: "https://cdn.remote.example/a.png", DeclaredMediaType: domain.MediaTypeImage},
	}}
	s := content.NewMediaService(content.MediaDeps{Repository: repo, Proxy: newProxyEnv(t).proxy})
	avatar := content.ImageRef{AccountID: ownerID, MediaID: remoteMP}
	header := content.ImageRef{AccountID: ownerID}

	got, err := s.Images(t.Context(), []content.ImageRef{avatar, header})
	if err != nil {
		t.Fatalf("Images(avatar, no header) = %v", err)
	}
	if _, ok := got[avatar]; !ok {
		t.Errorf("Images(avatar, no header) has no avatar: %v", got)
	}
	if _, ok := got[header]; ok {
		t.Errorf("Images(avatar, no header) has a header: %v", got)
	}
	if want := [][]string{{remoteMP}}; !slices.EqualFunc(repo.remote, want, slices.Equal) {
		t.Errorf("ListRemoteMedia calls = %q, want %q", repo.remote, want)
	}

	repo.remote = nil
	got, err = s.Images(t.Context(), []content.ImageRef{header})
	if err != nil || len(got) != 0 || len(repo.remote) != 0 {
		t.Errorf("Images(no header) = %v, %v with ListRemoteMedia calls %q, want no image, no error, and no call", got, err, repo.remote)
	}
}
