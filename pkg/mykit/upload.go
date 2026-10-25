// SPDX-License-Identifier: MIT

package mykit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// MediaFile is a file to upload.
type MediaFile struct {
	// Name is the file name, such as "photo.jpg".
	Name string
	// ContentType is the file's media type, such as "image/jpeg"; empty sends application/octet-stream. The
	// server checks it against the content.
	ContentType string
	// Body is the content, read once.
	Body io.Reader
}

// MediaUploadInput is what UploadMedia uploads.
type MediaUploadInput struct {
	// File is the image, video, or audio file.
	File MediaFile
	// Thumbnail is the poster of a video or audio file; nil for none.
	Thumbnail *MediaFile
	// Description is the alternative text.
	Description string
	// Purpose picks the variants that the server makes; empty means attachment.
	Purpose MediaUploadPurpose
}

// UploadMedia uploads a media file (uploadMedia), streaming the multipart body from the readers. The answer is
// the file in its current state, usually processing: poll GetMediaWithResponse until it is ready or failed.
// The body is read once, so the upload is never sent again: not after a 401 (get a fresh token first, as the
// TokenSource does when the token has expired) and not after a 429 or 503.
func (a *API) UploadMedia(ctx context.Context, in MediaUploadInput) (*MediaAttachment, error) {
	if in.File.Body == nil {
		return nil, errors.New("mykit: UploadMedia: File.Body is nil")
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		pw.CloseWithError(writeMediaUpload(mw, in))
	}()
	resp, err := a.UploadMediaWithBodyWithResponse(ctx, mw.FormDataContentType(), pr)
	// Stop the writer when the request ended before it read the whole body.
	_ = pr.CloseWithError(io.ErrClosedPipe)
	if err != nil {
		return nil, err
	}
	if resp.JSON202 == nil {
		return nil, fmt.Errorf("mykit: UploadMedia: unexpected answer %d", resp.StatusCode())
	}
	return resp.JSON202, nil
}

// writeMediaUpload writes the multipart body of in and closes mw.
func writeMediaUpload(mw *multipart.Writer, in MediaUploadInput) error {
	if err := writeFilePart(mw, "file", in.File); err != nil {
		return err
	}
	if in.Thumbnail != nil {
		if in.Thumbnail.Body == nil {
			return errors.New("mykit: UploadMedia: Thumbnail.Body is nil")
		}
		if err := writeFilePart(mw, "thumbnail", *in.Thumbnail); err != nil {
			return err
		}
	}
	if in.Description != "" {
		if err := mw.WriteField("description", in.Description); err != nil {
			return err
		}
	}
	if in.Purpose != "" {
		if err := mw.WriteField("purpose", string(in.Purpose)); err != nil {
			return err
		}
	}
	return mw.Close()
}

// quoteEscaper escapes a file name in a Content-Disposition parameter, as mime/multipart does.
var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"", "\r", "%0D", "\n", "%0A")

func writeFilePart(mw *multipart.Writer, field string, f MediaFile) error {
	name := f.Name
	if name == "" {
		name = field
	}
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, quoteEscaper.Replace(name)))
	h.Set("Content-Type", ct)
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f.Body)
	return err
}
