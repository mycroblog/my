// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"errors"
	"reflect"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// mediaRecordsRemoteMedia checks RecordRemoteMedia and FindRemoteMedia: a remote file is
// recorded once per owner and URL with every declared value and its purpose (an attachment
// unless told otherwise), an upload is not a remote file,
// and an unknown ID is not found.
func mediaRecordsRemoteMedia(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	in := appcontent.RemoteMediaInput{
		OwnerAccountID: mediaOwner, RemoteURL: "https://remote.example/files/a.png",
		DeclaredContentType: "image/png", DeclaredMediaType: content.MediaTypeImage, Purpose: content.PurposeHeader,
		Width: 640, Height: 480, Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
		Focus: &content.MediaFocus{X: -0.5, Y: 0.25}, Description: "a cat",
	}
	record := func(in appcontent.RemoteMediaInput) content.RemoteMedia {
		t.Helper()
		var m content.RemoteMedia
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			m, err = e.service.RecordRemoteMedia(ctx, tx, in)
			return err
		})
		if err != nil {
			t.Fatalf("RecordRemoteMedia: %v", err)
		}
		return m
	}
	first := record(in)
	if first.MediaID == "" || first.Focus == nil || first.Focus.MediaID != first.MediaID || first.Purpose != content.PurposeHeader {
		t.Fatalf("RecordRemoteMedia = %+v", first)
	}
	found, err := e.h.Media.FindRemoteMedia(ctx, first.MediaID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(found, first) {
		t.Errorf("FindRemoteMedia = %+v, want %+v", found, first)
	}
	file, err := e.h.Media.FindMediaFile(ctx, first.MediaID)
	if err != nil || file.Origin != content.MediaOriginRemote || file.OwnerAccountID != mediaOwner {
		t.Errorf("FindMediaFile = %+v, %v; want a remote file of the owner", file, err)
	}

	if again := record(in); again.MediaID != first.MediaID {
		t.Errorf("recording the same URL again = %s, want %s", again.MediaID, first.MediaID)
	}
	bare := record(appcontent.RemoteMediaInput{OwnerAccountID: mediaOwner, RemoteURL: "https://remote.example/files/b"})
	if bare.MediaID == first.MediaID {
		t.Error("another URL got the same media file")
	}
	got, err := e.h.Media.FindRemoteMedia(ctx, bare.MediaID)
	want := content.RemoteMedia{
		MediaID: bare.MediaID, OwnerAccountID: mediaOwner, RemoteURL: "https://remote.example/files/b",
		DeclaredMediaType: content.MediaTypeUnknown, Purpose: content.PurposeAttachment,
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("FindRemoteMedia(undeclared) = %+v, %v; want %+v", got, err, want)
	}

	upload := e.upload(t, testkit.FixturePNG(4, 3, false))
	for _, id := range []string{upload.File.ID, "0192f0c1-0000-7000-8000-00000000dead"} {
		if _, err := e.h.Media.FindRemoteMedia(ctx, id); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindRemoteMedia(%s) = %v, want ErrNotFound", id, err)
		}
	}

	// A page of files is read together, by ID, leaving out uploads and unknown IDs.
	listed, err := e.h.Media.ListRemoteMedia(ctx, []string{bare.MediaID, upload.File.ID, first.MediaID, "0192f0c1-0000-7000-8000-00000000dead"})
	wantList := []content.RemoteMedia{first, want}
	if first.MediaID > bare.MediaID {
		wantList = []content.RemoteMedia{want, first}
	}
	if err != nil || !reflect.DeepEqual(listed, wantList) {
		t.Errorf("ListRemoteMedia = %+v, %v; want %+v", listed, err, wantList)
	}
}
