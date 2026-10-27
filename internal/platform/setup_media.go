// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"

	blobfs "github.com/mycroblog/my/internal/adapter/blob/fs"
	"github.com/mycroblog/my/internal/adapter/media/native"
	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/media"
	mediahttp "github.com/mycroblog/my/internal/interface/media"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpMedia builds the blob store, the media processor, the media proxy, and the media use
// cases with the custom emoji.
func (a *App) setUpMedia(cfg *config.Config, medias content.MediaRepository, linkCards content.LinkCardMediaReader,
	emojis content.CustomEmojiRepository,
) error {
	if err := a.setBlobs(cfg); err != nil {
		return err
	}
	if err := a.setMediaProcessor(cfg); err != nil {
		return err
	}
	// The media service presents remote files with the proxy's signed URLs, so the proxy comes first.
	var err error
	if a.MediaProxy, err = newMediaProxy(cfg, medias, linkCards, a); err != nil {
		return err
	}
	return a.setMedia(cfg, medias, emojis)
}

// setUpMediaHTTP registers the HTTP handlers of the media files and the media proxy.
func (a *App) setUpMediaHTTP() {
	mediahttp.New(mediahttp.Options{Blobs: a.Blobs, Problem: a.writeStatusProblem, Logger: a.Logger, Errors: a.ErrorReporter}).Register(surfaceRouter{a, surfaceMedia})
	mediahttp.NewProxy(mediahttp.ProxyOptions{Media: a.MediaProxy, Problem: a.writeStatusProblem, Logger: a.Logger, Errors: a.ErrorReporter}).Register(surfaceRouter{a, surfaceMedia})
}

// setMediaProcessor builds the media processor, which the media proxy and the media use cases
// share.
func (a *App) setMediaProcessor(cfg *config.Config) error {
	switch cfg.Media {
	case "native":
		a.MediaProcessor = native.New(native.Options{})
	default:
		return fmt.Errorf("media %q is not implemented yet", cfg.Media)
	}
	return nil
}

// setMedia builds the media use cases, and registers their jobs and schedules.
func (a *App) setMedia(cfg *config.Config, repo content.MediaRepository, emojis content.CustomEmojiRepository) error {
	a.Media = content.NewMediaService(content.MediaDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Blobs: a.Blobs, Processor: a.MediaProcessor,
		Outbox: a.Outbox, Jobs: a.Jobs, Clock: a.Clock, IDs: a.IDs,
		Limits: media.Limits{
			MaxBytes: cfg.MediaMaxImageBytes, MaxSide: cfg.MediaMaxImageSide, MaxPixels: cfg.MediaMaxImagePixels,
		},
		AVLimits: media.AVLimits{
			MaxBytes: cfg.MediaMaxAVBytes, MaxVideoDuration: cfg.MediaMaxVideoDuration, MaxAudioDuration: cfg.MediaMaxAudioDuration,
		},
		UnattachedTTL: cfg.MediaUnattachedTTL,
		// Avatars and headers refer to media files from the account module, the instance's images
		// from the instance module, and the pending custom emoji requests to their images
		// (ADR-0070 §2).
		Referrers: []content.MediaReferrer{a.Accounts, a.Instance, content.PendingCustomEmojiRequestMedia(emojis)},
		Proxy:     a.MediaProxy,
		Logger:    a.Logger,
	})
	a.Media.RegisterJobs(a.Handlers, a.Scheduler)
	a.CustomEmojis = content.NewCustomEmojiService(content.CustomEmojiDeps{
		UnitOfWork: a.UnitOfWork, Repository: emojis, Media: a.Media, Clock: a.Clock, IDs: a.IDs,
		MaxBytes: cfg.CustomEmojiMaxBytes,
		Creators: emojiCreators{a}, Quota: emojiQuota{a}, InstanceAccount: a.emojiImageOwner,
	})
	return nil
}

// setBlobs builds the blob store, which the media proxy's cache and the media use cases share.
func (a *App) setBlobs(cfg *config.Config) error {
	switch cfg.Blob {
	case "fs":
		store, err := blobfs.New(blobfs.Options{Dir: cfg.BlobFSDir, BaseURL: cfg.MediaBaseURL()})
		if err != nil {
			return err
		}
		a.Blobs, a.closeBlobs = sysmetrics.Blobs(store, a.Meter, "blob."+cfg.Blob), store.Close
	default:
		return fmt.Errorf("blob %q is not implemented yet", cfg.Blob)
	}
	return nil
}

// mediaProxyKeyInfo is the HKDF info that derives the media proxy's URL signing key from a KEK.
const mediaProxyKeyInfo = "my media proxy url signature v1"

// newMediaProxy builds the media proxy. Its HMAC keys are derived from the key-encryption keys
// with HKDF-SHA256, so that it needs no secret of its own: the current KEK signs, and the
// previous ones still verify the URLs signed before a rotation.
func newMediaProxy(cfg *config.Config, repo content.MediaRepository, cards content.LinkCardMediaReader, a *App) (*content.MediaProxy, error) {
	var keys [][]byte
	for _, kek := range append([][]byte{cfg.KEK}, cfg.PreviousKEKs...) {
		if len(kek) == 0 {
			continue
		}
		k, err := hkdf.Key(sha256.New, kek, nil, mediaProxyKeyInfo, sha256.Size)
		if err != nil {
			return nil, fmt.Errorf("derive the media proxy key: %w", err)
		}
		keys = append(keys, k)
	}
	p, err := content.NewMediaProxy(content.ProxyDeps{
		Repository: repo, HTTP: a.HTTPClient, Accounts: a.Accounts,
		Policy: a.ModerationPolicy,
		Settings: func(ctx context.Context) (content.ProxySettings, error) {
			s, err := a.Instance.Settings(ctx)
			return content.ProxySettings{
				Policy: content.RemoteMediaPolicy(s.RemoteMediaPolicy()), CacheTTL: s.RemoteMediaCacheTTL(),
			}, err
		},
		// The account module's avatars and headers are what the avatars policy keeps.
		UnitOfWork: a.UnitOfWork, Blobs: a.Blobs, Clock: a.Clock, ProfileImages: a.Accounts,
		MaxCacheSize: cfg.RemoteMediaCacheMaxBytes, EmojiMaxBytes: cfg.RemoteCustomEmojiMaxBytes, Processor: a.MediaProcessor,
		Keys: keys, BaseURL: cfg.MediaBaseURL(), Logger: a.Logger,
		// The federation mode decides which remote files are fetched (ADR-0076).
		Federation: a.mediaGate(),
		// The images of link cards are remote files without an owner (ADR-0062 §3).
		LinkCardMedia: cards,
	})
	if err != nil {
		return nil, err
	}
	p.RegisterJobs(a.Handlers, a.Scheduler)
	return p, nil
}
