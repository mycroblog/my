// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

// A Feature is an identifier of Instance.features: something a client may need to detect on a
// server, such as an optional area of the API that the instance can have off. A change of the
// client API that adds such a thing adds a Feature here, to features.txt, and to the description
// of Instance.features. An identifier is never removed or reused while /api/v1 lasts: clients
// detect features by it and ignore the ones they do not know.
type Feature struct {
	// ID is the identifier, in snake_case (^[a-z][a-z0-9_]*$).
	ID string
	// Meaning says in one line what a client may rely on while the identifier is listed.
	Meaning string
	// offered reports whether the server offers the feature now, from the rest of the Instance
	// information that GET /instance presents.
	offered func(in *Instance) bool
}

// features is the registry of every identifier, in the order Instance.features lists them.
var features = []Feature{
	{ID: "voice_rooms", Meaning: "the voice chat rooms (/voice/...) answer", offered: func(in *Instance) bool { return in.VoiceEnabled }},
	{ID: "full_text_search", Meaning: "searchStatuses searches the statuses", offered: func(in *Instance) bool { return in.FullTextSearch.Available }},
	{ID: "supporters", Meaning: "the supporter program shows its plans and takes subscriptions", offered: func(in *Instance) bool { return in.SupportersEnabled }},
	{ID: "tips", Meaning: "the server takes one-time tips to its operation", offered: func(in *Instance) bool { return in.TipsEnabled }},
	{ID: "web_push", Meaning: "Web Push subscriptions deliver notifications, with push_server_key", offered: func(in *Instance) bool { return in.PushServerKey != nil }},
	{ID: "translation", Meaning: "translateStatus translates through a provider of this server", offered: func(in *Instance) bool { return len(in.Translation.Providers) > 0 }},
}

// Features returns every identifier that Instance.features can list, in the order it lists them,
// whether this server offers it now or not.
func Features() []Feature {
	out := make([]Feature, len(features))
	copy(out, features)
	return out
}

// offeredFeatures returns the identifiers of the features that in tells of as offered: never nil,
// so that GET /instance gives an empty list rather than null.
func offeredFeatures(in *Instance) []string {
	out := []string{}
	for _, f := range features {
		if f.offered(in) {
			out = append(out, f.ID)
		}
	}
	return out
}
