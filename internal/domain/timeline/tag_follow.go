// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import "errors"

// MaxFollowedTags is the most hashtags an account follows (ADR-0013 §27). It bounds the IN of
// the hashtag branch of a home timeline's fan-out-on-read, one index range per tag, and what a
// screen can handle; it is a constant, not a setting.
const MaxFollowedTags = 100

// ErrTooManyFollowedTags reports a followed hashtag beyond MaxFollowedTags.
var ErrTooManyFollowedTags = errors.New("timeline: too many followed hashtags")
