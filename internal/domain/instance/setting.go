// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrUnknownKey reports a setting or text key that the code does not define.
var ErrUnknownKey = errors.New("instance: unknown key")

// ErrInvalidValue reports a setting value or text body that its key rejects.
var ErrInvalidValue = errors.New("instance: invalid value")

// SettingKey names an instance setting (data-model §4.9, instance_setting_revisions.key).
// Every key holds one scalar; a compound setting is split into several keys.
type SettingKey string

// Setting keys. Adding one means adding its Definition below and its value to the CHECK
// constraint of instance_setting_revisions.key in schema/<dialect>/09-instance.sql.
const (
	SettingRegistrationMode        SettingKey = "registration_mode"
	SettingMaxStatusLength         SettingKey = "max_status_length"
	SettingDefaultLocale           SettingKey = "default_locale"
	SettingLocalTimelineAccess     SettingKey = "local_timeline_access"
	SettingFederatedTimelineAccess SettingKey = "federated_timeline_access"
	SettingPreviewImage            SettingKey = "preview_image"
	// SettingContactAccount and SettingContactEmail are how people reach the instance's operators,
	// and SettingThumbnail is the image that represents the instance (its alt text is
	// TextThumbnailAlt): the about page, GET /instance, and NodeInfo publish them.
	SettingContactAccount      SettingKey = "contact_account"
	SettingContactEmail        SettingKey = "contact_email"
	SettingThumbnail           SettingKey = "thumbnail"
	SettingRemoteMediaPolicy   SettingKey = "remote_media_policy"
	SettingRemoteMediaCacheTTL SettingKey = "remote_media_cache_ttl"
	SettingAuthorizedFetch     SettingKey = "authorized_fetch"
	// SettingDomainActionsVisibility and SettingDomainActionsRationaleVisibility decide who
	// may read the public list of domain actions and their public comments (FR-718).
	SettingDomainActionsVisibility          SettingKey = "domain_actions_visibility"
	SettingDomainActionsRationaleVisibility SettingKey = "domain_actions_rationale_visibility"
	// SettingAppealWindow is how many days after an action on an account the account may
	// appeal it (FR-707, ADR-0038).
	SettingAppealWindow SettingKey = "appeal_window"
	// The retention periods of the moderation records, in days (ADR-0038): the sign-in and
	// registration origins, the reports after their last resolution, the actions on accounts
	// and their appeals after the action ended, and the audit log.
	SettingRetentionSessionOrigins      SettingKey = "retention_session_origins"
	SettingRetentionRegistrationOrigins SettingKey = "retention_registration_origins"
	SettingRetentionReports             SettingKey = "retention_reports"
	SettingRetentionModerationActions   SettingKey = "retention_moderation_actions"
	SettingRetentionAuditLog            SettingKey = "retention_audit_log"
	// SettingSearchIndexing, SettingAIUsage, and SettingIndexableDefault are what the instance
	// states to search engines and AI crawlers, and the indexable default of new accounts
	// (FR-725, ADR-0057). Unset, they take the deny side.
	SettingSearchIndexing   SettingKey = "search_indexing"
	SettingAIUsage          SettingKey = "ai_usage"
	SettingIndexableDefault SettingKey = "indexable_default"
	// SettingSecurityContact is where vulnerabilities are reported (/.well-known/security.txt,
	// RFC 9116); empty serves no security.txt.
	SettingSecurityContact SettingKey = "security_contact"
	// SettingLinkCardsEnabled is whether statuses show the cards of their links, fetched from
	// the linked pages (FR-117, ADR-0062).
	SettingLinkCardsEnabled SettingKey = "link_cards_enabled"
	// SettingDynamicClientRegistration is whether OAuth clients may register themselves
	// without authentication at POST /oauth/register (RFC 7591, ADR-0090 §1).
	SettingDynamicClientRegistration SettingKey = "dynamic_client_registration"
	// SettingRequireMediaDescription makes every media file attached to a local post need an
	// alt text (FR-105).
	SettingRequireMediaDescription SettingKey = "require_media_description"
	// SettingFinanceVisibility decides how much of the instance's costs the public API and the
	// about page show (FR-903, ADR-0067 §8).
	SettingFinanceVisibility SettingKey = "finance_visibility"
	// SettingFinanceViewerItemNames, SettingFinanceViewerItemMemos, and SettingFinanceViewerIncome
	// decide how much of the instance's finances the holders of view_finance read on /finance
	// (ADR-0070 §5): the names of the cost items and the services of the imported costs, the
	// memos of the cost items, and the income as totals only or with the members of each plan.
	SettingFinanceViewerItemNames SettingKey = "finance_viewer_item_names"
	SettingFinanceViewerItemMemos SettingKey = "finance_viewer_item_memos"
	SettingFinanceViewerIncome    SettingKey = "finance_viewer_income"
	// SettingSupporterListVisibility decides who may read the supporter list of the members who
	// chose to be listed (ADR-0071 §4).
	SettingSupporterListVisibility SettingKey = "supporter_list_visibility"
	// SettingIncomeVisibility decides how much of the instance's income the public API and the
	// about page show (FR-917, ADR-0073 §7), apart from SettingFinanceVisibility.
	SettingIncomeVisibility SettingKey = "income_visibility"
	// SettingFederationMode is the direction the instance federates in (FR-413, ADR-0076).
	SettingFederationMode SettingKey = "federation_mode"
	// SettingUserAppRegistration is whether users may register OAuth apps (ADR-0090 §1).
	SettingUserAppRegistration SettingKey = "user_app_registration"
	// SettingOAuthMetadataDocumentClients is whether OAuth clients may use the https URL of a
	// Client ID Metadata Document as their client_id without registering (ADR-0090 §1).
	SettingOAuthMetadataDocumentClients SettingKey = "oauth_metadata_document_clients"
	// SettingVoiceRoomStarters decides who may open voice chat rooms (ADR-0100 §2),
	// SettingVoicePublicSummary whether the page of a local room shows its summary to visitors
	// who are not signed in to this server (§9), and SettingRetentionVoiceParticipants how many
	// days the records of the participants of a room are kept after it ended (§13).
	SettingVoiceRoomStarters          SettingKey = "voice_room_starters"
	SettingVoicePublicSummary         SettingKey = "voice_public_summary"
	SettingRetentionVoiceParticipants SettingKey = "retention_voice_participants"
)

// RegistrationMode is the value of SettingRegistrationMode (FR-001).
type RegistrationMode string

// Registration modes.
const (
	RegistrationOpen     RegistrationMode = "open"     // anyone may sign up
	RegistrationApproval RegistrationMode = "approval" // sign-ups wait for a moderator's approval
	RegistrationInvite   RegistrationMode = "invite"   // sign-up needs an invite
	RegistrationClosed   RegistrationMode = "closed"   // nobody may sign up
)

// TimelineAccess is the value of SettingLocalTimelineAccess and SettingFederatedTimelineAccess
// (FR-202): who may read the timeline.
type TimelineAccess string

// Timeline access values.
const (
	TimelineOff      TimelineAccess = "off"       // nobody: the API is a 404
	TimelineLoggedIn TimelineAccess = "logged_in" // the instance's logged-in users
	TimelineEveryone TimelineAccess = "everyone"  // everyone, anonymous visitors included
)

var timelineAccessChoices = []string{string(TimelineOff), string(TimelineLoggedIn), string(TimelineEveryone)}

// FederationMode is the value of SettingFederationMode (FR-413, ADR-0076). Its values are those
// of the federation module's mode (federation.Mode), which decides what each one lets through.
type FederationMode string

// Federation modes.
const (
	FederationOff      FederationMode = "off"      // federates in neither direction
	FederationInbound  FederationMode = "inbound"  // follows and reads remote accounts, sends nothing out
	FederationOutbound FederationMode = "outbound" // delivers to remote followers, takes nothing in
	FederationBoth     FederationMode = "both"     // federates in both directions
)

// RemoteMediaPolicy is the value of SettingRemoteMediaPolicy (FR-409, ADR-0005): which remote
// media files the instance keeps a copy of.
type RemoteMediaPolicy string

// Remote media policies.
const (
	RemoteMediaNone    RemoteMediaPolicy = "none"    // keep nothing; the media proxy relays every file
	RemoteMediaAvatars RemoteMediaPolicy = "avatars" // keep avatars and headers for remote_media_cache_ttl
	RemoteMediaAll     RemoteMediaPolicy = "all"     // keep every remote file for remote_media_cache_ttl
)

// Audience is the value of the settings that decide who may read something public about the
// instance (SettingDomainActionsVisibility, SettingDomainActionsRationaleVisibility).
type Audience string

// Audiences.
const (
	AudienceNone  Audience = "none"  // nobody
	AudienceUsers Audience = "users" // the instance's logged-in users
	AudienceAll   Audience = "all"   // everyone, anonymous visitors included
)

// Allows reports whether a reader may read what a reaches; authenticated is whether the reader
// is a logged-in user of the instance.
func (a Audience) Allows(authenticated bool) bool {
	return a == AudienceAll || (a == AudienceUsers && authenticated)
}

var audienceChoices = []string{string(AudienceNone), string(AudienceUsers), string(AudienceAll)}

// FinanceVisibility is the value of SettingFinanceVisibility (ADR-0067 §8): how much of the
// instance's costs everyone may read. No level publishes the names, memos, or individual payments
// of the cost items.
type FinanceVisibility string

// Finance visibilities, from the least to the most revealing.
const (
	FinancePrivate   FinanceVisibility = "private"   // nothing: the public API is a 404
	FinanceTotals    FinanceVisibility = "totals"    // the total of each currency by period
	FinanceBreakdown FinanceVisibility = "breakdown" // the totals and their breakdown by provider and category
)

// FinanceViewerIncome is the value of SettingFinanceViewerIncome (ADR-0070 §5): how much of the
// income the holders of view_finance read. No value shows the amounts of each plan or who paid.
type FinanceViewerIncome string

// Finance viewer income values.
const (
	FinanceViewerIncomeTotals FinanceViewerIncome = "totals" // the total of each currency by period
	FinanceViewerIncomePlans  FinanceViewerIncome = "plans"  // the totals and the number of members of each plan
)

// IncomeVisibility is the value of SettingIncomeVisibility (ADR-0073 §7): how much of the
// instance's income everyone may read. No level publishes the individual payments, the
// subscribers, the amounts of each plan, or the gross and the fees.
type IncomeVisibility string

// Income visibilities, from the least to the most revealing.
const (
	IncomePrivate IncomeVisibility = "private" // nothing: the public API is a 404
	IncomeTotals  IncomeVisibility = "totals"  // the net total of each currency by period
	IncomePlans   IncomeVisibility = "plans"   // the totals and the number of members of each plan
)

// SupporterListVisibility is the value of SettingSupporterListVisibility (ADR-0071 §4): who may
// read the supporter list.
type SupporterListVisibility string

// Supporter list visibilities.
const (
	SupporterListOff      SupporterListVisibility = "off"       // nobody: the API is a 404
	SupporterListPublic   SupporterListVisibility = "public"    // everyone
	SupporterListSignedIn SupporterListVisibility = "signed_in" // the users signed in to this server
)

// VoiceRoomStarters is the value of SettingVoiceRoomStarters (ADR-0100 §2): who may open voice
// chat rooms among the local users who may host them.
type VoiceRoomStarters string

// Voice room starters values.
const (
	VoiceRoomStartersEveryone  VoiceRoomStarters = "everyone"  // every signed-in local user
	VoiceRoomStartersPermitted VoiceRoomStarters = "permitted" // the holders of start_voice_rooms
)

// FinanceViewer is what the holders of view_finance read of the instance's finances beyond the
// totals and their breakdown by provider and category, which they always read (ADR-0070 §5).
type FinanceViewer struct {
	ItemNames bool
	ItemMemos bool
	Income    FinanceViewerIncome
}

// SearchIndexing is the value of SettingSearchIndexing (ADR-0057): whether search engines may
// index the instance's pages.
type SearchIndexing string

// Search indexing values.
const (
	SearchAllow SearchIndexing = "allow"
	SearchDeny  SearchIndexing = "deny"
)

// AIUsage is the value of SettingAIUsage (ADR-0057): what AI may do with the instance's pages.
type AIUsage string

// AI usage values, from the least to the most restrictive.
const (
	AIAllow        AIUsage = "allow"         // no reservation
	AIDenyTraining AIUsage = "deny_training" // no training of AI models
	AIDeny         AIUsage = "deny"          // no training, and no use by AI (AI search, assistants, agents)
)

// Bounds of SettingRemoteMediaCacheTTL, in seconds: a minute to a year.
const (
	MinRemoteMediaCacheTTL = 60
	MaxRemoteMediaCacheTTL = 365 * 24 * 60 * 60
)

// Bounds of SettingMaxStatusLength, in characters.
const (
	MinMaxStatusLength = 1
	MaxMaxStatusLength = 100000
)

// Bounds and default of SettingAppealWindow, in days (ADR-0038).
const (
	MinAppealWindow     = 20
	MaxAppealWindow     = 365
	DefaultAppealWindow = 180
)

// Floors and defaults of the retention settings, in days (ADR-0038). A period may not be set
// shorter than its floor; MaxRetention bounds every period.
const (
	MinRetentionOrigins        = 90
	DefaultRetentionOrigins    = 180
	MinRetentionModeration     = 180
	DefaultRetentionModeration = 365
	MinRetentionAuditLog       = 365
	DefaultRetentionAuditLog   = 730
	MaxRetention               = 3650
	// The records of the participants of voice chat rooms (ADR-0100 §13).
	MinRetentionVoiceParticipants     = 30
	DefaultRetentionVoiceParticipants = 90
)

// Kind is the scalar type of a setting's value.
type Kind string

// Kinds of setting values.
const (
	KindBool   Kind = "bool"   // "true" or "false"
	KindInt    Kind = "int"    // a decimal integer in [Min, Max]
	KindEnum   Kind = "enum"   // one of Choices
	KindLocale Kind = "locale" // a canonical BCP 47 language tag
	// KindMedia is the ID of a media file (a lowercase UUID), or "" for none.
	KindMedia Kind = "media"
	// KindAccount is the username of a local account, lowercase, or "" for none.
	KindAccount Kind = "account"
	// KindEmail is an email address (a bare addr-spec such as alice@example.com), or "" for none.
	KindEmail Kind = "email"
	// KindURI is an https: or mailto: URI, or "" for none.
	KindURI Kind = "uri"
)

// MaxURILength bounds a KindURI value, in bytes.
const MaxURILength = 2048

// Definition describes a setting key: its type, allowed values, and default. A key that has
// never been set reads as its default.
type Definition struct {
	Key     SettingKey
	Kind    Kind
	Default string
	// Min and Max bound a KindInt value, inclusive.
	Min, Max int
	// Choices are the values of a KindEnum key.
	Choices []string
	Doc     string
}

var definitions = []Definition{
	{
		Key: SettingRegistrationMode, Kind: KindEnum, Default: string(RegistrationApproval),
		Choices: []string{string(RegistrationOpen), string(RegistrationApproval), string(RegistrationInvite), string(RegistrationClosed)},
		Doc:     "Who may sign up (FR-001).",
	},
	{
		Key: SettingMaxStatusLength, Kind: KindInt, Default: "500", Min: MinMaxStatusLength, Max: MaxMaxStatusLength,
		Doc: "Maximum length of a post, in characters (FR-101).",
	},
	{
		Key: SettingDefaultLocale, Kind: KindLocale, Default: "en",
		Doc: "Initial UI locale of new users, and the locale for visitors whose preferences match no catalog (ADR-0009).",
	},
	{
		Key: SettingLocalTimelineAccess, Kind: KindEnum, Default: string(TimelineEveryone), Choices: timelineAccessChoices,
		Doc: "Who may read the local timeline: nobody, logged-in users, or everyone (FR-202).",
	},
	{
		Key: SettingFederatedTimelineAccess, Kind: KindEnum, Default: string(TimelineEveryone), Choices: timelineAccessChoices,
		Doc: "Who may read the federated timeline: nobody, logged-in users, or everyone (FR-202).",
	},
	{
		Key: SettingPreviewImage, Kind: KindMedia, Default: "",
		Doc: "Media ID of the link preview image of profiles and posts without images; empty means the built-in image (ADR-0028).",
	},
	{
		Key: SettingContactAccount, Kind: KindAccount, Default: "",
		Doc: "Username of the local account that people contact about the instance, shown on the about page, in GET /instance, and as NodeInfo's maintainer; empty means none.",
	},
	{
		Key: SettingContactEmail, Kind: KindEmail, Default: "",
		Doc: "Email address that people contact about the instance, shown on the about page, in GET /instance, and as NodeInfo's maintainer; empty means none.",
	},
	{
		Key: SettingThumbnail, Kind: KindMedia, Default: "",
		Doc: "Media ID of the image that represents the instance on the about and landing pages, in GET /instance, and in NodeInfo; its alt text is the thumbnail_alt text; empty means none.",
	},
	{
		Key: SettingRemoteMediaPolicy, Kind: KindEnum, Default: string(RemoteMediaNone),
		Choices: []string{string(RemoteMediaNone), string(RemoteMediaAvatars), string(RemoteMediaAll)},
		Doc:     "Which remote media files the instance keeps a copy of; none relays every file through the media proxy (FR-409, ADR-0005).",
	},
	{
		Key: SettingRemoteMediaCacheTTL, Kind: KindInt, Default: "86400", Min: MinRemoteMediaCacheTTL, Max: MaxRemoteMediaCacheTTL,
		Doc: "How long a fetched remote media file may be kept, in seconds: the max-age of the media proxy's responses for the CDN and browsers (FR-409).",
	},
	{
		Key: SettingAuthorizedFetch, Kind: KindBool, Default: "false",
		Doc: "Whether ActivityPub objects, collections, and full actor documents are served only to requests with a verified HTTP signature (authorized fetch, FR-408).",
	},
	{
		Key: SettingDomainActionsVisibility, Kind: KindEnum, Default: string(AudienceNone), Choices: audienceChoices,
		Doc: "Who may read the list of the limited and suspended domains: nobody, logged-in users, or everyone (FR-718).",
	},
	{
		Key: SettingDomainActionsRationaleVisibility, Kind: KindEnum, Default: string(AudienceNone), Choices: audienceChoices,
		Doc: "Who may read the public comments of the listed domain actions: nobody, logged-in users, or everyone (FR-718).",
	},
	{
		Key: SettingAppealWindow, Kind: KindInt, Default: strconv.Itoa(DefaultAppealWindow), Min: MinAppealWindow, Max: MaxAppealWindow,
		Doc: "How many days after a moderation action a local account may appeal it, once (FR-707, ADR-0038).",
	},
	{
		Key: SettingRetentionSessionOrigins, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionOrigins), Min: MinRetentionOrigins, Max: MaxRetention,
		Doc: "How many days the IP address and User-Agent of a sign-in are kept after the session started (ADR-0038).",
	},
	{
		Key: SettingRetentionRegistrationOrigins, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionOrigins), Min: MinRetentionOrigins, Max: MaxRetention,
		Doc: "How many days the IP address of a registration is kept after it (ADR-0038).",
	},
	{
		Key: SettingRetentionReports, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionModeration), Min: MinRetentionModeration, Max: MaxRetention,
		Doc: "How many days a report is kept after its last resolution; a report that an action follows from is kept as long as the action (ADR-0038).",
	},
	{
		Key: SettingRetentionModerationActions, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionModeration), Min: MinRetentionModeration, Max: MaxRetention,
		Doc: "How many days an action on an account and its appeal are kept after the action ended: its lift, expiry, or the decision on its appeal, whichever is last (ADR-0038).",
	},
	{
		Key: SettingRetentionAuditLog, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionAuditLog), Min: MinRetentionAuditLog, Max: MaxRetention,
		Doc: "How many days an audit log entry is kept (ADR-0037, ADR-0038).",
	},
	{
		Key: SettingSearchIndexing, Kind: KindEnum, Default: string(SearchDeny),
		Choices: []string{string(SearchAllow), string(SearchDeny)},
		Doc:     "Whether search engines may index the profiles and public posts of the accounts that allow it (indexable); deny states noindex on every page (FR-725, ADR-0057).",
	},
	{
		Key: SettingAIUsage, Kind: KindEnum, Default: string(AIDeny),
		Choices: []string{string(AIAllow), string(AIDenyTraining), string(AIDeny)},
		Doc:     "What the instance states to AI crawlers: allow, deny_training (no training of AI models), or deny (no training and no use by AI search, assistants, and agents) (FR-725, ADR-0057).",
	},
	{
		Key: SettingDynamicClientRegistration, Kind: KindBool, Default: "true",
		Doc: "Whether OAuth clients may register themselves at POST /oauth/register without authentication (RFC 7591); when off, the endpoint refuses every request (ADR-0090 §1).",
	},
	{
		Key: SettingIndexableDefault, Kind: KindBool, Default: "false",
		Doc: "The indexable setting of new accounts; existing accounts keep theirs (FR-412, ADR-0057).",
	},
	{
		Key: SettingSecurityContact, Kind: KindURI, Default: "",
		Doc: "Where to report vulnerabilities, an https: or mailto: URI; /.well-known/security.txt is served only when it is set (RFC 9116, ADR-0057).",
	},
	{
		Key: SettingRequireMediaDescription, Kind: KindBool, Default: "false",
		Doc: "Whether every media file attached to a local post needs an alt text; posts and edits without one are rejected (FR-105).",
	},
	{
		Key: SettingOAuthMetadataDocumentClients, Kind: KindBool, Default: "true",
		Doc: "Whether OAuth clients may use the https URL of a Client ID Metadata Document as their client_id; the server fetches the document instead of a registration (ADR-0090 §1).",
	},
	{
		Key: SettingLinkCardsEnabled, Kind: KindBool, Default: "true",
		Doc: "Whether statuses show the cards of their links, which the server fetches from the linked pages (FR-117, ADR-0062).",
	},
	{
		Key: SettingFinanceVisibility, Kind: KindEnum, Default: string(FinancePrivate),
		Choices: []string{string(FinancePrivate), string(FinanceTotals), string(FinanceBreakdown)},
		Doc:     "How much of the instance's costs GET /instance/cost_totals and the about page show: private (nothing), totals (the total of each currency by period), or breakdown (also by provider and category); item names, memos, and individual payments are never shown (FR-903, ADR-0067 §8).",
	},
	{
		Key: SettingFinanceViewerItemNames, Kind: KindBool, Default: "false",
		Doc: "Whether the holders of the view_finance permission read the names of the cost items and the services of the imported costs on /finance (ADR-0070 §5).",
	},
	{
		Key: SettingFinanceViewerItemMemos, Kind: KindBool, Default: "false",
		Doc: "Whether the holders of the view_finance permission read the memos of the cost items on /finance (ADR-0070 §5).",
	},
	{
		Key: SettingFinanceViewerIncome, Kind: KindEnum, Default: string(FinanceViewerIncomeTotals),
		Choices: []string{string(FinanceViewerIncomeTotals), string(FinanceViewerIncomePlans)},
		Doc:     "How much of the income the holders of the view_finance permission read on /finance: totals (the total of each currency by period) or plans (also the number of members of each plan, never its amounts) (ADR-0070 §5).",
	},
	{
		Key: SettingSupporterListVisibility, Kind: KindEnum, Default: string(SupporterListOff),
		Choices: []string{string(SupporterListOff), string(SupporterListPublic), string(SupporterListSignedIn)},
		Doc:     "Who may read the supporter list (GET /instance/supporters and /about/supporters) of the members of the supporter plans who chose to be listed: off (nobody), public (everyone), or signed_in (the users signed in to this server); it shows their accounts and plans only, never amounts or dates (ADR-0071 §4).",
	},
	{
		Key: SettingIncomeVisibility, Kind: KindEnum, Default: string(IncomePrivate),
		Choices: []string{string(IncomePrivate), string(IncomeTotals), string(IncomePlans)},
		Doc:     "How much of the instance's income GET /instance/income_totals and the about page show: private (nothing), totals (the net income of each currency by period), or plans (also the number of members of each supporter plan now); the balance against the costs is shown only while finance_visibility is not private as well, and individual payments, subscribers, the amounts of each plan, and the fees are never shown (FR-917, ADR-0073 §7).",
	},
	{
		Key: SettingFederationMode, Kind: KindEnum, Default: string(FederationBoth),
		Choices: []string{string(FederationOff), string(FederationInbound), string(FederationOutbound), string(FederationBoth)},
		Doc:     "The direction the instance federates in: off (neither), inbound (follows and reads remote accounts but sends none of the local posts, profiles, and reactions out), outbound (takes remote followers and delivers the local posts to them but takes none of the remote posts and reactions in), or both; narrowing it keeps the follows, the posts taken in, and the delivery records (FR-413, ADR-0076).",
	},
	{
		Key: SettingUserAppRegistration, Kind: KindBool, Default: "true",
		Doc: "Whether signed-in users may register their own OAuth apps, public or confidential, through the client API; apps registered earlier keep working when it is off (ADR-0090 §1).",
	},
	{
		Key: SettingVoiceRoomStarters, Kind: KindEnum, Default: string(VoiceRoomStartersPermitted),
		Choices: []string{string(VoiceRoomStartersEveryone), string(VoiceRoomStartersPermitted)},
		Doc:     "Who may open voice chat rooms: everyone (every signed-in local user) or permitted (only the holders of the start_voice_rooms permission) (ADR-0100 §2).",
	},
	{
		Key: SettingVoicePublicSummary, Kind: KindBool, Default: "false",
		Doc: "Whether the page of a voice chat room whose visibility is local shows its name, description, host, and whether it is open to visitors who are not signed in to this server, and in its link preview; its participants are never shown (ADR-0100 §9).",
	},
	{
		Key: SettingRetentionVoiceParticipants, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionVoiceParticipants), Min: MinRetentionVoiceParticipants, Max: MaxRetention,
		Doc: "How many days the records of who took part in a voice chat room, and as what, are kept after the room ended; the records of a reported room are kept as long as the report (ADR-0100 §13).",
	},
}

// Definitions returns the definitions of every setting key, in a fixed order.
func Definitions() []Definition {
	out := slices.Clone(definitions)
	for i := range out {
		out[i].Choices = slices.Clone(out[i].Choices)
	}
	return out
}

// Lookup returns the definition of key, or an error wrapping ErrUnknownKey.
func Lookup(key SettingKey) (Definition, error) {
	for _, d := range definitions {
		if d.Key == key {
			return d, nil
		}
	}
	return Definition{}, fmt.Errorf("setting %q: %w", key, ErrUnknownKey)
}

// Parse validates raw as a value of d and returns its canonical string form, which is what the
// database stores: "true"/"false" for KindBool and a decimal without sign or leading zeros
// for KindInt. A KindLocale value must already be canonical (the application layer
// canonicalizes it with internal/i18n); Parse checks only its shape.
func (d Definition) Parse(raw string) (string, error) {
	invalid := func(why string) error {
		return fmt.Errorf("setting %s: %q %s: %w", d.Key, raw, why, ErrInvalidValue)
	}
	switch d.Kind {
	case KindBool:
		switch strings.ToLower(raw) {
		case "true":
			return "true", nil
		case "false":
			return "false", nil
		}
		return "", invalid("is not true or false")
	case KindInt:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return "", invalid("is not an integer")
		}
		if n < d.Min || n > d.Max {
			return "", invalid(fmt.Sprintf("is outside %d..%d", d.Min, d.Max))
		}
		return strconv.Itoa(n), nil
	case KindEnum:
		if slices.Contains(d.Choices, raw) {
			return raw, nil
		}
		return "", invalid("is not one of " + strings.Join(d.Choices, ", "))
	case KindLocale:
		if !wellFormedTag(raw) {
			return "", invalid("is not a BCP 47 language tag")
		}
		return raw, nil
	case KindMedia:
		id := strings.ToLower(raw)
		if id != "" && !wellFormedUUID(id) {
			return "", invalid("is not a media ID")
		}
		return id, nil
	case KindAccount:
		if raw != "" && !wellFormedUsername(raw) {
			return "", invalid("is not a username")
		}
		return strings.ToLower(raw), nil
	case KindEmail:
		if raw != "" && !wellFormedEmail(raw) {
			return "", invalid("is not an email address")
		}
		return raw, nil
	case KindURI:
		if raw == "" {
			return "", nil
		}
		if !wellFormedContactURI(raw) {
			return "", invalid("is not an https: or mailto: URI")
		}
		return raw, nil
	}
	return "", invalid("has an unknown kind " + string(d.Kind))
}

// wellFormedTag reports whether tag consists of hyphen-separated subtags of 1 to 8 ASCII
// letters or digits, starting with a 2 or 3 letter language subtag.
func wellFormedTag(tag string) bool {
	parts := strings.Split(tag, "-")
	if l := len(parts[0]); l < 2 || l > 3 {
		return false
	}
	for i, p := range parts {
		if p == "" || len(p) > 8 {
			return false
		}
		for _, r := range p {
			letter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
			if !letter && (i == 0 || r < '0' || r > '9') {
				return false
			}
		}
	}
	return true
}

// maxUsernameLength is the longest local username, in bytes (account.MaxUsernameLength).
const maxUsernameLength = 30

// wellFormedUsername reports whether name has the shape of a local username: 1 to 30 ASCII
// letters, digits, and underscores (account.ValidateUsername).
func wellFormedUsername(name string) bool {
	if name == "" || len(name) > maxUsernameLength {
		return false
	}
	for i := range len(name) {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// MaxEmailLength is the longest value of a KindEmail setting, in bytes (identity.MaxEmailLength).
const MaxEmailLength = 320

// wellFormedEmail reports whether address is a bare addr-spec (local@domain) of at most
// MaxEmailLength bytes of valid UTF-8 without NUL, without a display name, angle brackets, or
// spaces around it (identity.ValidateEmail).
func wellFormedEmail(address string) bool {
	if len(address) > MaxEmailLength || !utf8.ValidString(address) || strings.ContainsRune(address, 0) {
		return false
	}
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Name == "" && parsed.Address == address
}

// wellFormedContactURI reports whether s is an absolute https: URI with a host or a mailto: URI
// with an address, at most MaxURILength bytes, without spaces or control characters.
func wellFormedContactURI(s string) bool {
	if len(s) > MaxURILength || strings.IndexFunc(s, func(r rune) bool { return r <= ' ' || r == 0x7f }) >= 0 {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != "" && u.User == nil
	case "mailto":
		return strings.Contains(u.Opaque, "@")
	}
	return false
}

// wellFormedUUID reports whether id is a UUID in lowercase hexadecimal, 8-4-4-4-12.
func wellFormedUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				return false
			}
		}
	}
	return true
}

// SettingRevision is one change of a setting (instance_setting_revisions). Value is canonical
// (Definition.Parse). The revision with the greatest ID is the current value of its key.
type SettingRevision struct {
	ID    string
	Key   SettingKey
	Value string
	// RevisedByUserID is the user who made the change, or "" for a change from the CLI.
	RevisedByUserID string
	RevisedAt       time.Time
}

// CurrentSetting is the current value of a key (the instance_settings projection).
type CurrentSetting struct {
	Key        SettingKey
	RevisionID string
	Value      string
}

// Supersedes reports whether r should replace the current value c, which is the zero value
// when the key has no current value yet. IDs are UUIDv7, so the later revision wins.
func (r SettingRevision) Supersedes(c CurrentSetting) bool {
	return c.RevisionID == "" || r.ID > c.RevisionID
}

// Settings are the values of every setting key: the stored value, or the default for a key
// that has never been set. The zero value reads every key as its default.
type Settings struct {
	stored map[SettingKey]string
}

// NewSettings returns the settings whose stored values are current. Values of unknown keys
// are ignored.
func NewSettings(current []CurrentSetting) Settings {
	s := Settings{stored: map[SettingKey]string{}}
	for _, c := range current {
		if _, err := Lookup(c.Key); err == nil {
			s.stored[c.Key] = c.Value
		}
	}
	return s
}

// Value returns the value of key and whether it is stored (false: the default applies).
// It returns "" and false for an unknown key.
func (s Settings) Value(key SettingKey) (value string, stored bool) {
	if v, ok := s.stored[key]; ok {
		return v, true
	}
	d, err := Lookup(key)
	if err != nil {
		return "", false
	}
	return d.Default, false
}

func (s Settings) boolValue(key SettingKey) bool {
	v, _ := s.Value(key)
	return v == "true"
}

// RegistrationMode returns the value of SettingRegistrationMode.
func (s Settings) RegistrationMode() RegistrationMode {
	v, _ := s.Value(SettingRegistrationMode)
	return RegistrationMode(v)
}

// intValue returns the value of a KindInt key, or its default when the stored value does not
// parse.
func (s Settings) intValue(key SettingKey) int {
	v, _ := s.Value(key)
	n, err := strconv.Atoi(v)
	if err != nil {
		d, _ := Lookup(key)
		n, _ = strconv.Atoi(d.Default)
	}
	return n
}

// MaxStatusLength returns the value of SettingMaxStatusLength.
func (s Settings) MaxStatusLength() int { return s.intValue(SettingMaxStatusLength) }

// DefaultLocale returns the value of SettingDefaultLocale.
func (s Settings) DefaultLocale() string {
	v, _ := s.Value(SettingDefaultLocale)
	return v
}

// LocalTimelineAccess returns the value of SettingLocalTimelineAccess.
func (s Settings) LocalTimelineAccess() TimelineAccess {
	v, _ := s.Value(SettingLocalTimelineAccess)
	return TimelineAccess(v)
}

// FederatedTimelineAccess returns the value of SettingFederatedTimelineAccess.
func (s Settings) FederatedTimelineAccess() TimelineAccess {
	v, _ := s.Value(SettingFederatedTimelineAccess)
	return TimelineAccess(v)
}

// PreviewImageMediaID returns the value of SettingPreviewImage, "" when unset.
func (s Settings) PreviewImageMediaID() string {
	v, _ := s.Value(SettingPreviewImage)
	return v
}

// ContactAccount returns the value of SettingContactAccount, "" when unset.
func (s Settings) ContactAccount() string {
	v, _ := s.Value(SettingContactAccount)
	return v
}

// ContactEmail returns the value of SettingContactEmail, "" when unset.
func (s Settings) ContactEmail() string {
	v, _ := s.Value(SettingContactEmail)
	return v
}

// ThumbnailMediaID returns the value of SettingThumbnail, "" when unset.
func (s Settings) ThumbnailMediaID() string {
	v, _ := s.Value(SettingThumbnail)
	return v
}

// RemoteMediaPolicy returns the value of SettingRemoteMediaPolicy.
func (s Settings) RemoteMediaPolicy() RemoteMediaPolicy {
	v, _ := s.Value(SettingRemoteMediaPolicy)
	return RemoteMediaPolicy(v)
}

// RemoteMediaCacheTTL returns the value of SettingRemoteMediaCacheTTL.
func (s Settings) RemoteMediaCacheTTL() time.Duration {
	return time.Duration(s.intValue(SettingRemoteMediaCacheTTL)) * time.Second
}

// AuthorizedFetch returns the value of SettingAuthorizedFetch.
func (s Settings) AuthorizedFetch() bool { return s.boolValue(SettingAuthorizedFetch) }

// FederationMode returns the value of SettingFederationMode.
func (s Settings) FederationMode() FederationMode {
	v, _ := s.Value(SettingFederationMode)
	return FederationMode(v)
}

// DomainActionsVisibility returns the value of SettingDomainActionsVisibility.
func (s Settings) DomainActionsVisibility() Audience {
	v, _ := s.Value(SettingDomainActionsVisibility)
	return Audience(v)
}

// DomainActionsRationaleVisibility returns the value of
// SettingDomainActionsRationaleVisibility.
func (s Settings) DomainActionsRationaleVisibility() Audience {
	v, _ := s.Value(SettingDomainActionsRationaleVisibility)
	return Audience(v)
}

// AppealWindow returns the value of SettingAppealWindow as a duration.
func (s Settings) AppealWindow() time.Duration {
	return time.Duration(s.intValue(SettingAppealWindow)) * 24 * time.Hour
}

// Retention returns the value of a retention setting (SettingRetentionSessionOrigins,
// SettingRetentionRegistrationOrigins, SettingRetentionReports,
// SettingRetentionModerationActions, SettingRetentionAuditLog,
// SettingRetentionVoiceParticipants) as a duration.
func (s Settings) Retention(key SettingKey) time.Duration {
	return time.Duration(s.intValue(key)) * 24 * time.Hour
}

// SearchIndexing returns the value of SettingSearchIndexing; an unexpected value denies.
func (s Settings) SearchIndexing() SearchIndexing {
	if v, _ := s.Value(SettingSearchIndexing); v == string(SearchAllow) {
		return SearchAllow
	}
	return SearchDeny
}

// AIUsage returns the value of SettingAIUsage; an unexpected value denies.
func (s Settings) AIUsage() AIUsage {
	switch v, _ := s.Value(SettingAIUsage); AIUsage(v) {
	case AIAllow:
		return AIAllow
	case AIDenyTraining:
		return AIDenyTraining
	}
	return AIDeny
}

// DynamicClientRegistration returns the value of SettingDynamicClientRegistration.
func (s Settings) DynamicClientRegistration() bool {
	return s.boolValue(SettingDynamicClientRegistration)
}

// OAuthMetadataDocumentClients returns the value of SettingOAuthMetadataDocumentClients.
func (s Settings) OAuthMetadataDocumentClients() bool {
	return s.boolValue(SettingOAuthMetadataDocumentClients)
}

// IndexableDefault returns the value of SettingIndexableDefault.
func (s Settings) IndexableDefault() bool { return s.boolValue(SettingIndexableDefault) }

// FinanceVisibility returns the value of SettingFinanceVisibility; an unexpected value is private.
func (s Settings) FinanceVisibility() FinanceVisibility {
	switch v, _ := s.Value(SettingFinanceVisibility); FinanceVisibility(v) {
	case FinanceTotals:
		return FinanceTotals
	case FinanceBreakdown:
		return FinanceBreakdown
	}
	return FinancePrivate
}

// IncomeVisibility returns the value of SettingIncomeVisibility; an unexpected value is private.
func (s Settings) IncomeVisibility() IncomeVisibility {
	switch v, _ := s.Value(SettingIncomeVisibility); IncomeVisibility(v) {
	case IncomeTotals:
		return IncomeTotals
	case IncomePlans:
		return IncomePlans
	}
	return IncomePrivate
}

// FinanceViewer returns the values of SettingFinanceViewerItemNames,
// SettingFinanceViewerItemMemos, and SettingFinanceViewerIncome; an unexpected income value is
// totals.
func (s Settings) FinanceViewer() FinanceViewer {
	income := FinanceViewerIncomeTotals
	if v, _ := s.Value(SettingFinanceViewerIncome); v == string(FinanceViewerIncomePlans) {
		income = FinanceViewerIncomePlans
	}
	return FinanceViewer{
		ItemNames: s.boolValue(SettingFinanceViewerItemNames), ItemMemos: s.boolValue(SettingFinanceViewerItemMemos), Income: income,
	}
}

// SupporterListVisibility returns the value of SettingSupporterListVisibility; an unexpected value
// is off.
func (s Settings) SupporterListVisibility() SupporterListVisibility {
	switch v, _ := s.Value(SettingSupporterListVisibility); SupporterListVisibility(v) {
	case SupporterListPublic:
		return SupporterListPublic
	case SupporterListSignedIn:
		return SupporterListSignedIn
	}
	return SupporterListOff
}

// VoiceRoomStarters returns the value of SettingVoiceRoomStarters; an unexpected value is
// permitted.
func (s Settings) VoiceRoomStarters() VoiceRoomStarters {
	if v, _ := s.Value(SettingVoiceRoomStarters); v == string(VoiceRoomStartersEveryone) {
		return VoiceRoomStartersEveryone
	}
	return VoiceRoomStartersPermitted
}

// VoicePublicSummary returns the value of SettingVoicePublicSummary.
func (s Settings) VoicePublicSummary() bool { return s.boolValue(SettingVoicePublicSummary) }

// LinkCardsEnabled returns the value of SettingLinkCardsEnabled.
func (s Settings) LinkCardsEnabled() bool { return s.boolValue(SettingLinkCardsEnabled) }

// UserAppRegistration returns the value of SettingUserAppRegistration.
func (s Settings) UserAppRegistration() bool { return s.boolValue(SettingUserAppRegistration) }

// SecurityContact returns the value of SettingSecurityContact, "" when unset.
func (s Settings) SecurityContact() string {
	v, _ := s.Value(SettingSecurityContact)
	return v
}

// RequireMediaDescription returns the value of SettingRequireMediaDescription.
func (s Settings) RequireMediaDescription() bool {
	return s.boolValue(SettingRequireMediaDescription)
}

// CrawlerPolicy returns what the instance states to crawlers on its own (ADR-0057).
func (s Settings) CrawlerPolicy() CrawlerPolicy {
	return CrawlerPolicy{Search: s.SearchIndexing(), AI: s.AIUsage()}
}

// MediaKeys returns the setting keys whose values are media IDs.
func MediaKeys() []SettingKey {
	var out []SettingKey
	for _, d := range definitions {
		if d.Kind == KindMedia {
			out = append(out, d.Key)
		}
	}
	return out
}
