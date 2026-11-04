// SPDX-License-Identifier: AGPL-3.0-only

package content

// StatusURI returns the ActivityPub id of a local status on host (ADR-0018).
func StatusURI(host, statusID string) string {
	return "https://" + host + "/ap/objects/" + statusID
}

// CustomEmojiURI returns the ActivityPub id of a local custom emoji on host (ADR-0051 §3); it is
// derived from the emoji's ID and never stored.
func CustomEmojiURI(host, emojiID string) string {
	return "https://" + host + "/ap/emojis/" + emojiID
}

// StatusPageSegment is the reserved word under a profile page that the page of one of its
// statuses lives under: /@{username}/status/{id} (ADR-0018).
const StatusPageSegment = "status"

// StatusURL returns the HTML page of a local status on host (ADR-0018). The page is found by
// the status ID alone; username only makes the URL readable.
func StatusURL(host, username, statusID string) string {
	return "https://" + host + "/@" + username + "/" + StatusPageSegment + "/" + statusID
}

// PageURL returns the HTML page of s, whose author's current username is authorUsername when
// s is local. A local status's page is derived on read from its author's current username, so
// that a rename or a change of the URL's form leaves no stale value behind (ADR-0018); a remote
// status has the url it arrived with, "" when it has none.
func (s Status) PageURL(host, authorUsername string) string {
	if s.IsLocal {
		return StatusURL(host, authorUsername, s.ID)
	}
	return s.URL
}
