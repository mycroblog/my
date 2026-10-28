// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/mycroblog/my/internal/app/instance"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/web"
)

// setUpWeb builds the web client's handler (ADR-0028), registers its assets and manifest, and
// sets a.page to its HTML handler.
func (a *App) setUpWeb(opts AppOptions) error {
	if opts.WebBuild == nil {
		opts.WebBuild = web.Embedded()
	}
	h, err := web.New(web.Options{
		Build: opts.WebBuild, DevViteOrigin: opts.DevViteOrigin,
		Host: a.Config.Host, AccountDomain: a.Config.AccountDomain, WebOrigin: a.Config.WebOrigin,
		// Images and media may come from the CDN in front of /media/ (media_origin).
		MediaOrigin: a.Config.MediaOrigin,
		// The instance's display name, read per request; the host when it has none.
		InstanceName: a.instanceName,
		// The landing page's link preview describes the instance by its summary, read per request.
		InstanceDescription: a.instanceSummary,
		// The HTML is the same for every visitor, so only the instance default applies.
		DefaultLocale: func(ctx context.Context) string { return a.Locales.Locale(ctx, "") },
		// Profile and post pages carry link previews of what an anonymous visitor may see.
		Previews: a.Previews, Messages: a.Messages,
		// The pages of the published documents carry link previews under their names.
		Documents: a.Instance,
		// The pages of the voice chat rooms carry the summaries the instance shows (ADR-0100 §9).
		VoiceRooms: a.voiceRoomSummary,
		// The instance's preview image is read per request, so that a change applies at once.
		DefaultImage: a.previewImage,
		// The instance theme (ADR-0059), read per request so that a saved theme applies at once.
		Theme:         a.Instance.Theme,
		ThemeRevision: a.themeRevision,
		// The instance's own icon and logos (ADR-0116), read per request so that a change applies at once.
		Brands:    a.Instance.Brands,
		BrandFile: a.brandFile,
		// What /robots.txt, the pages, and /.well-known/ state to crawlers (ADR-0057), read per request.
		Settings: a.Instance.Settings,
		Now:      a.Clock.Now,
		NotFound: func(w http.ResponseWriter, r *http.Request) { a.writeStatusProblem(w, r, http.StatusNotFound) },
		Logger:   a.Logger,
	})
	if err != nil {
		return err
	}
	h.Register(surfaceRouter{a, surfaceWeb})
	a.webStylesheet = h.Stylesheet()
	a.webPasskeyScript = h.PasskeySignInScript()
	// The ActivityPub handler wraps it for the requests that ask for ActivityPub (activitypub.Handler.Pages).
	a.page = h
	return nil
}

// themeRevision returns a theme revision for its stylesheet, and false when there is none.
func (a *App) themeRevision(ctx context.Context, id string) (domaininstance.ThemeRevision, bool, error) {
	rev, err := a.Instance.ThemeRevision(ctx, id)
	if errors.Is(err, instance.ErrNotFound) {
		return domaininstance.ThemeRevision{}, false, nil
	}
	return rev, err == nil, err
}

// brandFile opens a file of a brand revision, and false when there is none.
func (a *App) brandFile(ctx context.Context, revisionID string, r domaininstance.BrandRendition) (domaininstance.BrandFile, io.ReadCloser, bool, error) {
	f, rc, err := a.Instance.BrandFile(ctx, revisionID, r)
	if errors.Is(err, instance.ErrNotFound) {
		return domaininstance.BrandFile{}, nil, false, nil
	}
	return f, rc, err == nil, err
}

// themeStylesheet returns the path of the current theme's stylesheet for the server-rendered
// pages, or "" when the theme changes nothing or cannot be read (the pages then show the defaults).
func (a *App) themeStylesheet(ctx context.Context) string {
	rev, ok, err := a.Instance.Theme(ctx)
	if err != nil {
		a.Logger.WarnContext(ctx, "read the instance theme", slog.Any("error", err))
		return ""
	}
	if !ok {
		return ""
	}
	return web.ThemeStylesheetPath(rev.ID)
}

// previewImage returns the instance's link preview image (the preview_image setting), or nil when it is unset or its media file is not a processed
// image, which leaves the web client's built-in image.
func (a *App) previewImage(ctx context.Context) (*web.Image, error) {
	setting, err := a.Instance.PreviewImage(ctx)
	if err != nil || setting.MediaID == "" {
		return nil, err
	}
	img, err := a.Previews.Image(ctx, setting.MediaID, setting.Alt)
	if err != nil || img == nil {
		return nil, err
	}
	return &web.Image{URL: img.URL, Width: img.Width, Height: img.Height, Alt: img.Alt}, nil
}
