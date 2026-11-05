// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"errors"
	"fmt"
	"maps"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
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
	SettingRegistrationMode            SettingKey = "registration_mode"
	SettingMaxStatusLength             SettingKey = "max_status_length"
	SettingDefaultLocale               SettingKey = "default_locale"
	SettingLocalTimelineVisibility     SettingKey = "local_timeline_visibility"
	SettingFederatedTimelineVisibility SettingKey = "federated_timeline_visibility"
	SettingPreviewImage                SettingKey = "preview_image"
	// SettingContactAccount and SettingContactEmail are how people reach the instance's operators,
	// and SettingThumbnail is the image that represents the instance (its alt text is
	// TextThumbnailAlt): the about page, GET /instance, and NodeInfo publish them.
	SettingContactAccount             SettingKey = "contact_account"
	SettingContactEmail               SettingKey = "contact_email"
	SettingThumbnail                  SettingKey = "thumbnail"
	SettingRemoteMediaPolicy          SettingKey = "remote_media_policy"
	SettingRemoteMediaCacheTTLSeconds SettingKey = "remote_media_cache_ttl_seconds"
	SettingAuthorizedFetch            SettingKey = "authorized_fetch"
	// SettingDomainActionsVisibility and SettingDomainActionsRationaleVisibility decide who
	// may read the public list of domain actions and their public comments (FR-718).
	SettingDomainActionsVisibility          SettingKey = "domain_actions_visibility"
	SettingDomainActionsRationaleVisibility SettingKey = "domain_actions_rationale_visibility"
	// SettingAppealWindowDays is how many days after an action on an account the account may
	// appeal it (FR-707, ADR-0058).
	SettingAppealWindowDays SettingKey = "appeal_window_days"
	// The retention periods of the moderation records, in days (ADR-0058): the sign-in and
	// registration origins, the reports after their last resolution, the actions on accounts
	// and their appeals after the action ended, and the audit log.
	SettingSessionOriginsRetentionDays      SettingKey = "session_origins_retention_days"
	SettingRegistrationOriginsRetentionDays SettingKey = "registration_origins_retention_days"
	SettingReportsRetentionDays             SettingKey = "reports_retention_days"
	SettingModerationActionsRetentionDays   SettingKey = "moderation_actions_retention_days"
	SettingAuditLogRetentionDays            SettingKey = "audit_log_retention_days"
	// SettingSearchIndexing, SettingAIUsage, and SettingIndexableDefault are what the instance
	// states to search engines and AI crawlers, and the indexable default of new accounts
	// (FR-725, ADR-0057). Unset, they take the deny side.
	SettingSearchIndexing   SettingKey = "search_indexing"
	SettingAIUsage          SettingKey = "ai_usage"
	SettingIndexableDefault SettingKey = "indexable_default"
	// SettingSecurityContact is where vulnerabilities are reported (/.well-known/security.txt,
	// RFC 9116); empty serves no security.txt.
	SettingSecurityContact SettingKey = "security_contact"
	// SettingLinkCards is whether statuses show the cards of their links, fetched from
	// the linked pages (FR-117, ADR-0062).
	SettingLinkCards SettingKey = "link_cards"
	// SettingLinkCardsIgnoreRobotsTxt is whether the fetches of link cards skip robots.txt
	// (ADR-0062 §12).
	SettingLinkCardsIgnoreRobotsTxt SettingKey = "link_cards_ignore_robots_txt"
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
	// chose to be listed (ADR-0070 §13).
	SettingSupporterListVisibility SettingKey = "supporter_list_visibility"
	// SettingIncomeVisibility decides how much of the instance's income the public API and the
	// about page show (FR-917, ADR-0067 §17), apart from SettingFinanceVisibility.
	SettingIncomeVisibility SettingKey = "income_visibility"
	// SettingFederationMode is the direction the instance federates in (FR-413, ADR-0006).
	SettingFederationMode SettingKey = "federation_mode"
	// SettingUserAppRegistration is whether users may register OAuth apps (ADR-0090 §1).
	SettingUserAppRegistration SettingKey = "user_app_registration"
	// SettingOAuthMetadataDocumentClients is whether OAuth clients may use the https URL of a
	// Client ID Metadata Document as their client_id without registering (ADR-0090 §1).
	SettingOAuthMetadataDocumentClients SettingKey = "oauth_metadata_document_clients"
	// SettingVoiceRoomStarters decides who may open voice chat rooms (ADR-0100 §2),
	// SettingVoicePublicSummary whether the page of a local room shows its summary to visitors
	// who are not signed in to this server (§9), and SettingVoiceParticipantsRetentionDays how many
	// days the records of the participants of a room are kept after it ended (§13).
	SettingVoiceRoomStarters              SettingKey = "voice_room_starters"
	SettingVoicePublicSummary             SettingKey = "voice_public_summary"
	SettingVoiceParticipantsRetentionDays SettingKey = "voice_participants_retention_days"
	// SettingInviteCreators decides who may create invites of their own (ADR-0106 §14), and
	// SettingMemberInviteMaxActive, SettingMemberInviteMaxUses, and
	// SettingMemberInviteMaxLifetimeSeconds bound those invites (§2).
	SettingInviteCreators                 SettingKey = "invite_creators"
	SettingMemberInviteMaxActive          SettingKey = "member_invite_max_active"
	SettingMemberInviteMaxUses            SettingKey = "member_invite_max_uses"
	SettingMemberInviteMaxLifetimeSeconds SettingKey = "member_invite_max_lifetime_seconds"
	// SettingPublicCounts decides whether GET /instance/overview, and so the landing page, shows
	// the number of members, the public posts of the last days, and when the instance started.
	SettingPublicCounts SettingKey = "public_counts"
	// SettingFeaturedAccounts are the local accounts that the suggestions of accounts show first,
	// in this order (FR-308).
	SettingFeaturedAccounts SettingKey = "featured_accounts"
	// SettingLoginProvidersOnly limits registration and sign-in to the external login providers
	// while at least one is enabled: passwords and passkeys no longer sign in. With no enabled
	// provider it does not apply (identity.Service.LoginProvidersOnly).
	SettingLoginProvidersOnly SettingKey = "login_providers_only"
	// SettingRemoteMediaCacheMaxBytes bounds the total size of the copies of remote media that
	// the media proxy keeps (FR-803).
	SettingRemoteMediaCacheMaxBytes SettingKey = "remote_media_cache_max_bytes"
	// SettingRemoteStatusRetentionDays is how many days a remote post that nothing local refers
	// to is kept after its publication, SettingRemoteStatusMaxRetentionDays bounds how long any
	// remote post is kept (0 for no bound, ADR-0006), and SettingRelayStatusRetentionDays is how
	// many days a post that only a relay brought in is kept (ADR-0006).
	SettingRemoteStatusRetentionDays    SettingKey = "remote_status_retention_days"
	SettingRemoteStatusMaxRetentionDays SettingKey = "remote_status_max_retention_days"
	SettingRelayStatusRetentionDays     SettingKey = "relay_status_retention_days"
	// SettingOAuthClientRateLimits caps the client API requests of OAuth clients, each across
	// all its users, for an administrator to hold back a client that overloads the server
	// (ADR-0090 §7).
	SettingOAuthClientRateLimits SettingKey = "oauth_client_rate_limits"
	// SettingTranslationProviders names the server-side translation providers that are enabled, in
	// the order of their priority; SettingTranslationProviderVisibilities says what each may
	// receive; and SettingTranslationBrowser enables the browser's built-in translation
	// (ADR-0148 §3, §5, §12). All start off.
	SettingTranslationProviders            SettingKey = "translation_providers"
	SettingTranslationProviderVisibilities SettingKey = "translation_provider_visibilities"
	SettingTranslationBrowser              SettingKey = "translation_browser"
	// SettingTranslationUserMonthlyCharacters and SettingTranslationServerMonthlyCharacters bound
	// the characters sent to providers in a UTC month. 0 sends nothing, and no value means
	// unlimited (ADR-0148 §11).
	SettingTranslationUserMonthlyCharacters   SettingKey = "translation_user_monthly_characters"
	SettingTranslationServerMonthlyCharacters SettingKey = "translation_server_monthly_characters"
	// SettingTranslationRetentionDays is how many days a translation is kept after it was made
	// (ADR-0148 §9).
	SettingTranslationRetentionDays SettingKey = "translation_retention_days"
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

// FederationMode is the value of SettingFederationMode (FR-413, ADR-0006). Its values are those
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
	RemoteMediaAvatars RemoteMediaPolicy = "avatars" // keep avatars and headers for remote_media_cache_ttl_seconds
	RemoteMediaAll     RemoteMediaPolicy = "all"     // keep every remote file for remote_media_cache_ttl_seconds
)

// Audience is the value of the settings that decide who may read something
// (SettingLocalTimelineVisibility, SettingFederatedTimelineVisibility,
// SettingDomainActionsVisibility, SettingDomainActionsRationaleVisibility, and
// SettingSupporterListVisibility).
type Audience string

// Audiences.
const (
	AudienceOff      Audience = "off"       // nobody
	AudienceSignedIn Audience = "signed_in" // the users signed in to this server
	AudienceEveryone Audience = "everyone"  // everyone, anonymous visitors included
)

// Allows reports whether a reader may read what a reaches; authenticated is whether the reader
// is a signed-in user of the instance.
func (a Audience) Allows(authenticated bool) bool {
	return a == AudienceEveryone || (a == AudienceSignedIn && authenticated)
}

var audienceChoices = []string{string(AudienceOff), string(AudienceSignedIn), string(AudienceEveryone)}

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

// IncomeVisibility is the value of SettingIncomeVisibility (ADR-0067 §17): how much of the
// instance's income everyone may read. No level publishes the individual payments, the
// subscribers, the amounts of each plan, or the gross and the fees.
type IncomeVisibility string

// Income visibilities, from the least to the most revealing.
const (
	IncomePrivate IncomeVisibility = "private" // nothing: the public API is a 404
	IncomeTotals  IncomeVisibility = "totals"  // the net total of each currency by period
	IncomePlans   IncomeVisibility = "plans"   // the totals and the number of members of each plan
)

// VoiceRoomStarters is the value of SettingVoiceRoomStarters (ADR-0100 §2): who may open voice
// chat rooms among the local users who may host them.
type VoiceRoomStarters string

// Voice room starters values.
const (
	VoiceRoomStartersEveryone  VoiceRoomStarters = "everyone"  // every signed-in local user
	VoiceRoomStartersPermitted VoiceRoomStarters = "permitted" // the holders of start_voice_rooms
)

// InviteCreators is the value of SettingInviteCreators (ADR-0106 §14): who may create invites of
// their own besides the holders of manage_invites.
type InviteCreators string

// Invite creators values.
const (
	InviteCreatorsEveryone  InviteCreators = "everyone"  // every signed-in local user
	InviteCreatorsPermitted InviteCreators = "permitted" // the holders of create_invites
)

// The bounds and defaults of the caps of the invites members create (ADR-0106 §15).
const (
	DefaultMemberInviteMaxActive    = 5
	MaxMemberInviteMaxActive        = 100
	DefaultMemberInviteMaxUses      = 10
	MaxMemberInviteMaxUses          = 1000
	DefaultMemberInviteMaxExpiresIn = 30 * 24 * time.Hour
	MinMemberInviteMaxExpiresIn     = time.Hour
	MaxMemberInviteMaxExpiresIn     = 365 * 24 * time.Hour
)

// MemberInviteCaps are the values of SettingMemberInviteMaxActive, SettingMemberInviteMaxUses,
// and SettingMemberInviteMaxLifetimeSeconds.
type MemberInviteCaps struct {
	MaxActive    int
	MaxUses      int
	MaxExpiresIn time.Duration
}

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

// Bounds of SettingRemoteMediaCacheTTLSeconds, in seconds: a minute to a year.
const (
	MinRemoteMediaCacheTTL = 60
	MaxRemoteMediaCacheTTL = 365 * 24 * 60 * 60
)

// Bounds of SettingMaxStatusLength, in characters.
const (
	MinMaxStatusLength = 1
	MaxMaxStatusLength = 100000
)

// Bounds and default of SettingAppealWindowDays, in days (ADR-0058).
const (
	MinAppealWindow     = 20
	MaxAppealWindow     = 365
	DefaultAppealWindow = 180
)

// Floors and defaults of the retention settings, in days (ADR-0058). A period may not be set
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
	// KindAccounts is the usernames of up to MaxSettingAccounts local accounts, lowercase,
	// separated by single spaces, without repeats, or "" for none.
	KindAccounts Kind = "accounts"
	// KindClientLimits is comma-separated client_id=requests pairs, or "" for none.
	KindClientLimits Kind = "client_limits"
	// KindProviderList is comma-separated translation provider IDs in order of priority, without
	// repeats, or "" for none.
	KindProviderList Kind = "provider_list"
	// KindProviderVisibilities is comma-separated provider=visibility|visibility pairs, sorted by
	// provider, or "" for none. A provider with no visibility after the "=" may receive nothing.
	KindProviderVisibilities Kind = "provider_visibilities"
)

// The defaults and bounds of the translation settings (ADR-0148 §9, §11).
const (
	DefaultTranslationUserMonthlyCharacters   = 50000
	DefaultTranslationServerMonthlyCharacters = 500000
	// MaxTranslationMonthlyCharacters is far above any bill an operator means to pay. It keeps a
	// value in range, and is not a way to say unlimited.
	MaxTranslationMonthlyCharacters = 1_000_000_000_000
	DefaultTranslationRetentionDays = 30
)

// TranslationVisibilities are the values a provider may be allowed, in their canonical order. They
// are the visibilities of posts (ADR-0017); a test of the platform keeps the two the same, since a
// domain imports no other module.
var TranslationVisibilities = []string{"public", "local", "unlisted", "private", "direct"}

// MaxClientLimitsLength bounds a KindClientLimits value, in bytes.
const MaxClientLimitsLength = 8192

// The bounds and defaults of the sizes and periods of remote content.
const (
	DefaultRemoteMediaCacheMaxBytes     = 1 << 30
	MaxRemoteMediaCacheMaxBytes         = 1 << 50
	DefaultRemoteStatusRetentionDays    = 90
	DefaultRemoteStatusMaxRetentionDays = 0
	DefaultRelayStatusRetentionDays     = 7
)

// MaxSettingAccounts bounds the usernames of a KindAccounts value.
const MaxSettingAccounts = 20

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
		Key: SettingLocalTimelineVisibility, Kind: KindEnum, Default: string(AudienceEveryone), Choices: audienceChoices,
		Doc: "Who may read the local timeline: nobody, signed-in users, or everyone (FR-202).",
	},
	{
		Key: SettingFederatedTimelineVisibility, Kind: KindEnum, Default: string(AudienceSignedIn), Choices: audienceChoices,
		Doc: "Who may read the federated timeline: nobody, signed-in users, or everyone (FR-202).",
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
		Key: SettingRemoteMediaCacheTTLSeconds, Kind: KindInt, Default: "86400", Min: MinRemoteMediaCacheTTL, Max: MaxRemoteMediaCacheTTL,
		Doc: "How long a fetched remote media file may be kept, in seconds: the max-age of the media proxy's responses for the CDN and browsers (FR-409).",
	},
	{
		Key: SettingAuthorizedFetch, Kind: KindBool, Default: "false",
		Doc: "Whether ActivityPub objects, collections, and full actor documents are served only to requests with a verified HTTP signature (authorized fetch, FR-408).",
	},
	{
		Key: SettingDomainActionsVisibility, Kind: KindEnum, Default: string(AudienceOff), Choices: audienceChoices,
		Doc: "Who may read the list of the limited and suspended domains: nobody, signed-in users, or everyone (FR-718).",
	},
	{
		Key: SettingDomainActionsRationaleVisibility, Kind: KindEnum, Default: string(AudienceOff), Choices: audienceChoices,
		Doc: "Who may read the public comments of the listed domain actions: nobody, signed-in users, or everyone (FR-718).",
	},
	{
		Key: SettingAppealWindowDays, Kind: KindInt, Default: strconv.Itoa(DefaultAppealWindow), Min: MinAppealWindow, Max: MaxAppealWindow,
		Doc: "How many days after a moderation action a local account may appeal it, once (FR-707, ADR-0058).",
	},
	{
		Key: SettingSessionOriginsRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionOrigins), Min: MinRetentionOrigins, Max: MaxRetention,
		Doc: "How many days the IP address and User-Agent of a sign-in are kept after the session started (ADR-0058).",
	},
	{
		Key: SettingRegistrationOriginsRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionOrigins), Min: MinRetentionOrigins, Max: MaxRetention,
		Doc: "How many days the IP address of a registration is kept after it (ADR-0058).",
	},
	{
		Key: SettingReportsRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionModeration), Min: MinRetentionModeration, Max: MaxRetention,
		Doc: "How many days a report is kept after its last resolution; a report that an action follows from is kept as long as the action (ADR-0058).",
	},
	{
		Key: SettingModerationActionsRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionModeration), Min: MinRetentionModeration, Max: MaxRetention,
		Doc: "How many days an action on an account and its appeal are kept after the action ended: its lift, expiry, or the decision on its appeal, whichever is last (ADR-0058).",
	},
	{
		Key: SettingAuditLogRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionAuditLog), Min: MinRetentionAuditLog, Max: MaxRetention,
		Doc: "How many days an audit log entry is kept (ADR-0058).",
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
		Key: SettingLinkCards, Kind: KindBool, Default: "true",
		Doc: "Whether statuses show the cards of their links, which the server fetches from the linked pages (FR-117, ADR-0062).",
	},
	{
		Key: SettingLinkCardsIgnoreRobotsTxt, Kind: KindBool, Default: "false",
		Doc: "Whether the fetches of link cards skip robots.txt, so that the linked sites cannot refuse them with it (ADR-0062).",
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
		Key: SettingSupporterListVisibility, Kind: KindEnum, Default: string(AudienceOff),
		Choices: audienceChoices,
		Doc:     "Who may read the supporter list (GET /instance/supporters and /about/supporters) of the members of the supporter plans who chose to be listed: off (nobody), public (everyone), or signed_in (the users signed in to this server); it shows their accounts and plans only, never amounts or dates (ADR-0070 §13).",
	},
	{
		Key: SettingIncomeVisibility, Kind: KindEnum, Default: string(IncomePrivate),
		Choices: []string{string(IncomePrivate), string(IncomeTotals), string(IncomePlans)},
		Doc:     "How much of the instance's income GET /instance/income_totals and the about page show: private (nothing), totals (the net income of each currency by period), or plans (also the number of members of each supporter plan now); the balance against the costs is shown only while finance_visibility is not private as well, and individual payments, subscribers, the amounts of each plan, and the fees are never shown (FR-917, ADR-0067 §17).",
	},
	{
		Key: SettingFederationMode, Kind: KindEnum, Default: string(FederationBoth),
		Choices: []string{string(FederationOff), string(FederationInbound), string(FederationOutbound), string(FederationBoth)},
		Doc:     "The direction the instance federates in: off (neither), inbound (follows and reads remote accounts but sends none of the local posts, profiles, and reactions out), outbound (takes remote followers and delivers the local posts to them but takes none of the remote posts and reactions in), or both; narrowing it keeps the follows, the posts taken in, and the delivery records (FR-413, ADR-0006).",
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
		Key: SettingVoiceParticipantsRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRetentionVoiceParticipants), Min: MinRetentionVoiceParticipants, Max: MaxRetention,
		Doc: "How many days the records of who took part in a voice chat room, and as what, are kept after the room ended; the records of a reported room are kept as long as the report (ADR-0100 §13).",
	},
	{
		Key: SettingInviteCreators, Kind: KindEnum, Default: string(InviteCreatorsPermitted),
		Choices: []string{string(InviteCreatorsEveryone), string(InviteCreatorsPermitted)},
		Doc:     "Who may create invites of their own besides the staff with manage_invites: everyone (every signed-in local user who is not suspended or limited) or permitted (only the holders of the create_invites permission) (ADR-0106 §14).",
	},
	{
		Key: SettingMemberInviteMaxActive, Kind: KindInt, Default: strconv.Itoa(DefaultMemberInviteMaxActive), Min: 0, Max: MaxMemberInviteMaxActive,
		Doc: "How many usable invites (not revoked, expired, or used up) one member may hold at once; 0 lets members create none. Invites of the staff do not count (ADR-0106 §15).",
	},
	{
		Key: SettingMemberInviteMaxUses, Kind: KindInt, Default: strconv.Itoa(DefaultMemberInviteMaxUses), Min: 1, Max: MaxMemberInviteMaxUses,
		Doc: "How many people may sign up with one invite that a member creates (ADR-0106 §15).",
	},
	{
		Key: SettingMemberInviteMaxLifetimeSeconds, Kind: KindInt, Default: strconv.Itoa(int(DefaultMemberInviteMaxExpiresIn / time.Second)),
		Min: int(MinMemberInviteMaxExpiresIn / time.Second), Max: int(MaxMemberInviteMaxExpiresIn / time.Second),
		Doc: "How long, in seconds, an invite that a member creates may work at most (ADR-0106 §15).",
	},
	{
		Key: SettingPublicCounts, Kind: KindBool, Default: "true",
		Doc: "Whether the landing page and GET /instance/overview show the number of members, the public posts of local accounts on each of the last 7 days, and when the instance started.",
	},
	{
		Key: SettingFeaturedAccounts, Kind: KindAccounts, Default: "",
		Doc: "Usernames of up to 20 local accounts, separated by spaces or commas, that the suggestions of accounts show first, in this order; an account that turned off being suggested is left out (FR-308).",
	},
	{
		Key: SettingRemoteMediaCacheMaxBytes, Kind: KindInt, Default: strconv.Itoa(DefaultRemoteMediaCacheMaxBytes),
		Min: 1, Max: MaxRemoteMediaCacheMaxBytes,
		Doc: "Size limit in bytes of the copies of remote media that the media proxy keeps; the hourly cleanup deletes the least recently accessed ones over it (FR-803).",
	},
	{
		Key: SettingRemoteStatusRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRemoteStatusRetentionDays), Min: 1, Max: MaxRetention,
		Doc: "How many days a remote post is kept after its publication when nothing on this instance refers to it (ADR-0006).",
	},
	{
		Key: SettingRemoteStatusMaxRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRemoteStatusMaxRetentionDays), Min: 0, Max: MaxRetention,
		Doc: "The most days any remote post is kept after its publication, even when this instance refers to it; 0 sets no limit (ADR-0006).",
	},
	{
		Key: SettingRelayStatusRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultRelayStatusRetentionDays), Min: 1, Max: MaxRetention,
		Doc: "How many days a remote post that only a relay brought in is kept after its publication (ADR-0006).",
	},
	{
		Key: SettingOAuthClientRateLimits, Kind: KindClientLimits, Default: "",
		Doc: "Caps on the client API requests of OAuth clients: comma-separated client_id=requests pairs, the requests that all the users of the client together may make in 5 minutes (ADR-0090 §7).",
	},
	{
		Key: SettingLoginProvidersOnly, Kind: KindBool, Default: "false",
		Doc: "Whether registration and sign-in are limited to the external login providers while at least one is enabled: passwords and passkeys no longer sign in, and users without a linked provider cannot sign in again once their sessions end. With no enabled provider it does not apply.",
	},
	{
		Key: SettingTranslationProviders, Kind: KindProviderList, Default: "",
		Doc: "The server-side translation providers that are enabled: comma-separated IDs, the first tried first. Empty sends nothing to any provider (ADR-0148 §3).",
	},
	{
		Key: SettingTranslationProviderVisibilities, Kind: KindProviderVisibilities, Default: "",
		Doc: "What each translation provider may receive: comma-separated provider=visibility|visibility pairs. A provider not named receives public, local, and unlisted posts; provider= receives nothing (ADR-0148 §5).",
	},
	{
		Key: SettingTranslationBrowser, Kind: KindBool, Default: "false",
		Doc: "Whether readers may use the translation built into their browser, which sends nothing to this server or to a provider (ADR-0148 §12).",
	},
	{
		Key: SettingTranslationUserMonthlyCharacters, Kind: KindInt, Default: strconv.Itoa(DefaultTranslationUserMonthlyCharacters), Min: 0, Max: MaxTranslationMonthlyCharacters,
		Doc: "How many characters of posts one user's requests may send to translation providers in a UTC month; 0 sends nothing (ADR-0148 §11).",
	},
	{
		Key: SettingTranslationServerMonthlyCharacters, Kind: KindInt, Default: strconv.Itoa(DefaultTranslationServerMonthlyCharacters), Min: 0, Max: MaxTranslationMonthlyCharacters,
		Doc: "How many characters of posts the whole server may send to translation providers in a UTC month; 0 sends nothing (ADR-0148 §11).",
	},
	{
		Key: SettingTranslationRetentionDays, Kind: KindInt, Default: strconv.Itoa(DefaultTranslationRetentionDays), Min: 1, Max: MaxRetention,
		Doc: "How many days a translation is kept after it was made; reading it does not extend this (ADR-0148 §9).",
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
	case KindClientLimits:
		limits, err := parseClientLimits(raw)
		if err != nil {
			return "", invalid(err.Error())
		}
		if len(raw) > MaxClientLimitsLength {
			return "", invalid(fmt.Sprintf("is over %d bytes", MaxClientLimitsLength))
		}
		return formatClientLimits(limits), nil
	case KindProviderList:
		ids, err := parseProviderList(raw)
		if err != nil {
			return "", invalid(err.Error())
		}
		return strings.Join(ids, ","), nil
	case KindProviderVisibilities:
		allowed, err := parseProviderVisibilities(raw)
		if err != nil {
			return "", invalid(err.Error())
		}
		return formatProviderVisibilities(allowed), nil
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
	case KindAccounts:
		names := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
		out := make([]string, 0, len(names))
		for _, n := range names {
			n = strings.ToLower(strings.TrimPrefix(n, "@"))
			if !wellFormedUsername(n) {
				return "", invalid("is not a list of usernames")
			}
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
		if len(out) > MaxSettingAccounts {
			return "", invalid(fmt.Sprintf("names more than %d accounts", MaxSettingAccounts))
		}
		return strings.Join(out, " "), nil
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

// LocalTimelineVisibility returns the value of SettingLocalTimelineVisibility.
func (s Settings) LocalTimelineVisibility() Audience {
	v, _ := s.Value(SettingLocalTimelineVisibility)
	return Audience(v)
}

// FederatedTimelineVisibility returns the value of SettingFederatedTimelineVisibility.
func (s Settings) FederatedTimelineVisibility() Audience {
	v, _ := s.Value(SettingFederatedTimelineVisibility)
	return Audience(v)
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

// RemoteMediaCacheTTL returns the value of SettingRemoteMediaCacheTTLSeconds.
func (s Settings) RemoteMediaCacheTTL() time.Duration {
	return time.Duration(s.intValue(SettingRemoteMediaCacheTTLSeconds)) * time.Second
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

// AppealWindow returns the value of SettingAppealWindowDays as a duration.
func (s Settings) AppealWindow() time.Duration {
	return time.Duration(s.intValue(SettingAppealWindowDays)) * 24 * time.Hour
}

// Retention returns the value of a retention setting (SettingSessionOriginsRetentionDays,
// SettingRegistrationOriginsRetentionDays, SettingReportsRetentionDays,
// SettingModerationActionsRetentionDays, SettingAuditLogRetentionDays,
// SettingVoiceParticipantsRetentionDays) as a duration.
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
func (s Settings) SupporterListVisibility() Audience {
	switch v, _ := s.Value(SettingSupporterListVisibility); Audience(v) {
	case AudienceEveryone:
		return AudienceEveryone
	case AudienceSignedIn:
		return AudienceSignedIn
	}
	return AudienceOff
}

// VoiceRoomStarters returns the value of SettingVoiceRoomStarters; an unexpected value is
// permitted.
func (s Settings) VoiceRoomStarters() VoiceRoomStarters {
	if v, _ := s.Value(SettingVoiceRoomStarters); v == string(VoiceRoomStartersEveryone) {
		return VoiceRoomStartersEveryone
	}
	return VoiceRoomStartersPermitted
}

// InviteCreators returns the value of SettingInviteCreators; an unexpected value is permitted.
func (s Settings) InviteCreators() InviteCreators {
	if v, _ := s.Value(SettingInviteCreators); v == string(InviteCreatorsEveryone) {
		return InviteCreatorsEveryone
	}
	return InviteCreatorsPermitted
}

// MemberInviteCaps returns the caps of the invites members create.
func (s Settings) MemberInviteCaps() MemberInviteCaps {
	return MemberInviteCaps{
		MaxActive:    s.intValue(SettingMemberInviteMaxActive),
		MaxUses:      s.intValue(SettingMemberInviteMaxUses),
		MaxExpiresIn: time.Duration(s.intValue(SettingMemberInviteMaxLifetimeSeconds)) * time.Second,
	}
}

// RemoteMediaCacheMaxBytes returns the value of SettingRemoteMediaCacheMaxBytes.
func (s Settings) RemoteMediaCacheMaxBytes() int64 {
	return int64(s.intValue(SettingRemoteMediaCacheMaxBytes))
}

// OAuthClientRateLimits returns the value of SettingOAuthClientRateLimits by client ID.
func (s Settings) OAuthClientRateLimits() map[string]int64 {
	v, _ := s.Value(SettingOAuthClientRateLimits)
	limits, err := parseClientLimits(v)
	if err != nil {
		return nil
	}
	return limits
}

// parseClientLimits parses client_id=requests pairs. The client_id of a metadata document is a
// URL that may hold "=", so each pair splits at its last "=".
func parseClientLimits(v string) (map[string]int64, error) {
	limits := map[string]int64{}
	for pair := range strings.SplitSeq(v, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		i := strings.LastIndex(pair, "=")
		if i <= 0 {
			return nil, errors.New("is not comma-separated client_id=requests pairs")
		}
		n, err := strconv.ParseInt(strings.TrimSpace(pair[i+1:]), 10, 64)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("has a cap of %q that is not a positive integer", pair[:i])
		}
		limits[strings.TrimSpace(pair[:i])] = n
	}
	return limits, nil
}

// formatClientLimits returns the stored form of limits, sorted by client ID.
func formatClientLimits(limits map[string]int64) string {
	pairs := make([]string, 0, len(limits))
	for _, id := range slices.Sorted(maps.Keys(limits)) {
		pairs = append(pairs, id+"="+strconv.FormatInt(limits[id], 10))
	}
	return strings.Join(pairs, ",")
}

// TranslationProviders returns the IDs of SettingTranslationProviders, the first tried first.
func (s Settings) TranslationProviders() []string {
	v, _ := s.Value(SettingTranslationProviders)
	ids, err := parseProviderList(v)
	if err != nil {
		return nil
	}
	return ids
}

// TranslationProviderVisibilities returns what SettingTranslationProviderVisibilities allows, by
// provider ID. A provider that it does not name is absent; one allowed nothing has an empty slice.
func (s Settings) TranslationProviderVisibilities() map[string][]string {
	v, _ := s.Value(SettingTranslationProviderVisibilities)
	allowed, err := parseProviderVisibilities(v)
	if err != nil {
		return nil
	}
	return allowed
}

// TranslationBrowser returns the value of SettingTranslationBrowser.
func (s Settings) TranslationBrowser() bool { return s.boolValue(SettingTranslationBrowser) }

// TranslationUserMonthlyCharacters returns the value of SettingTranslationUserMonthlyCharacters.
func (s Settings) TranslationUserMonthlyCharacters() int64 {
	return int64(s.intValue(SettingTranslationUserMonthlyCharacters))
}

// TranslationServerMonthlyCharacters returns the value of SettingTranslationServerMonthlyCharacters.
func (s Settings) TranslationServerMonthlyCharacters() int64 {
	return int64(s.intValue(SettingTranslationServerMonthlyCharacters))
}

// TranslationRetentionDays returns the value of SettingTranslationRetentionDays.
func (s Settings) TranslationRetentionDays() int {
	return s.intValue(SettingTranslationRetentionDays)
}

// maxProviderSettingLength bounds the values of KindProviderList and KindProviderVisibilities.
const maxProviderSettingLength = 4096

// wellFormedProviderID reports whether id can be the ID of a provider: a lowercase letter or digit,
// then up to 63 lowercase letters, digits, "_", or "-". A setting may name a provider that this
// server does not have, which then does nothing.
func wellFormedProviderID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i, r := range id {
		alnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !alnum && (i == 0 || (r != '_' && r != '-')) {
			return false
		}
	}
	return true
}

// parseProviderList parses comma-separated provider IDs, keeping their order and dropping repeats.
func parseProviderList(v string) ([]string, error) {
	if len(v) > maxProviderSettingLength {
		return nil, fmt.Errorf("is over %d bytes", maxProviderSettingLength)
	}
	var ids []string
	for id := range strings.SplitSeq(v, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !wellFormedProviderID(id) {
			return nil, fmt.Errorf("has %q, which is not a provider ID", id)
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// parseProviderVisibilities parses comma-separated provider=visibility|visibility pairs. The
// visibilities of a pair come back in the order of TranslationVisibilities, without repeats.
func parseProviderVisibilities(v string) (map[string][]string, error) {
	if len(v) > maxProviderSettingLength {
		return nil, fmt.Errorf("is over %d bytes", maxProviderSettingLength)
	}
	allowed := map[string][]string{}
	for pair := range strings.SplitSeq(v, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		id, list, ok := strings.Cut(pair, "=")
		id = strings.TrimSpace(id)
		if !ok || !wellFormedProviderID(id) {
			return nil, errors.New("is not comma-separated provider=visibility|visibility pairs")
		}
		named := map[string]bool{}
		for part := range strings.SplitSeq(list, "|") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !slices.Contains(TranslationVisibilities, part) {
				return nil, fmt.Errorf("has %q, which is not a visibility", part)
			}
			named[part] = true
		}
		visibilities := []string{}
		for _, name := range TranslationVisibilities {
			if named[name] {
				visibilities = append(visibilities, name)
			}
		}
		allowed[id] = visibilities
	}
	return allowed, nil
}

// formatProviderVisibilities returns the stored form of allowed, sorted by provider.
func formatProviderVisibilities(allowed map[string][]string) string {
	pairs := make([]string, 0, len(allowed))
	for _, id := range slices.Sorted(maps.Keys(allowed)) {
		pairs = append(pairs, id+"="+strings.Join(allowed[id], "|"))
	}
	return strings.Join(pairs, ",")
}

// LoginProvidersOnly returns the value of SettingLoginProvidersOnly: what the administrator chose,
// which applies only while a login provider is enabled.
func (s Settings) LoginProvidersOnly() bool { return s.boolValue(SettingLoginProvidersOnly) }

// PublicCounts returns the value of SettingPublicCounts.
func (s Settings) PublicCounts() bool { return s.boolValue(SettingPublicCounts) }

// FeaturedAccounts returns the usernames of SettingFeaturedAccounts, in order; none when unset.
func (s Settings) FeaturedAccounts() []string {
	v, _ := s.Value(SettingFeaturedAccounts)
	return strings.Fields(v)
}

// VoicePublicSummary returns the value of SettingVoicePublicSummary.
func (s Settings) VoicePublicSummary() bool { return s.boolValue(SettingVoicePublicSummary) }

// LinkCardsEnabled returns the value of SettingLinkCards.
func (s Settings) LinkCardsEnabled() bool { return s.boolValue(SettingLinkCards) }

// LinkCardsIgnoreRobotsTxt returns the value of SettingLinkCardsIgnoreRobotsTxt.
func (s Settings) LinkCardsIgnoreRobotsTxt() bool {
	return s.boolValue(SettingLinkCardsIgnoreRobotsTxt)
}

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
