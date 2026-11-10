// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import "errors"

// MaxFeaturedTags is the most hashtags an account features on its profile (ADR-0006 §17, the
// limit of Mastodon's FeaturedTag): a local account's own list, and what the fetch of a remote
// account's featuredTags collection keeps.
const MaxFeaturedTags = 10

// ErrTooManyFeaturedTags reports a list of featured hashtags beyond MaxFeaturedTags.
var ErrTooManyFeaturedTags = errors.New("timeline: too many featured hashtags")
