<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->

# Decision hook protocol

The services, messages, and enums of the package `my.plugin.hook.v1`, which [`hook.proto`](../../api/plugin/my/plugin/hook/v1/hook.proto) defines.

The decision hooks: the server asks a plugin whether to accept one subject before it does. The
plugin returns a verdict from a fixed vocabulary. It rewrites nothing, and it takes no action on
an account. Every hook has its own destination, and one plugin may serve several of them.

## Services

### `HookService`

#### `Check`

```proto
rpc Check(CheckRequest) returns (CheckResponse);
```

Request: [`CheckRequest`](#checkrequest). Response: [`CheckResponse`](#checkresponse).

Check returns the verdict on one subject. The server calls it again for the same subject_key
only when an earlier call failed, so a plugin should give the same answer again.

## Messages

### `CheckRequest`

- `hook` (`string`, field 1): The hook that asks: inbound.status, inbound.follow, signup.check, media.check, local.status, or
  local.profile.
- `subject_key` (`string`, field 2): What identifies the subject for the hook, such as the URI of a received activity.
- `probe` (`bool`, field 3): True for a call of `my plugin test`, which is about no real subject and acts on nothing.
- `inbound_status` ([`InboundStatus`](#inboundstatus), field 4): What inbound.status reviews; empty on the calls of the other hooks and on probes.
- `follow` ([`FollowSubject`](#followsubject), field 5): The two parties of the follow, for inbound.follow.
- `signup` ([`SignupSubject`](#signupsubject), field 6): What signup.check reviews; empty on the calls of the other hooks and on probes.
- `local_status` ([`LocalStatus`](#localstatus), field 7): What local.status reviews; empty on the calls of the other hooks and on probes.
- `media` ([`MediaSubject`](#mediasubject), field 8): What media.check reviews; empty on the calls of the other hooks and on probes.
- `local_profile` ([`LocalProfile`](#localprofile), field 10): What local.profile reviews; empty on the calls of the other hooks and on probes.

### `InboundStatus`

A post from another server, or a new revision of one, for inbound.status. It holds the post as
that server sent it. No field holds anything of a local post or of a local account, and a post
is sent only when it is related to a local account: a local account follows its author, it names a
local account, or it is in a thread with a local post.

- `activity_type` (`string`, field 1): The type of the activity that carried the post: Create or Update. Every revision is its own subject.
- `status_uri` (`string`, field 2)
- `status_url` (`string`, field 3): The address of the post for a person; empty when it is the same as status_uri.
- `author_uri` (`string`, field 4)
- `author_domain` (`string`, field 5)
- `visibility` (`string`, field 6): public, unlisted, private, or direct.
- `any_local_follower` (`bool`, field 7): Whether a local account follows the author.
- `in_reply_to_uri` (`string`, field 8): The post this one replies to; empty for none. The parent's text is not sent.
- `spoiler_text` (`string`, field 9)
- `sensitive` (`bool`, field 10): Whether the author marked the post as sensitive.
- `published_at` (`string`, field 11): When the author published it, in RFC 3339; empty when the post does not say.
- `bodies` (repeated [`InboundStatus.Body`](#inboundstatusbody), field 12)
- `attachments` (repeated [`InboundStatus.Attachment`](#inboundstatusattachment), field 13)
- `poll_options` (repeated `string`, field 14)
- `hashtags` (repeated `string`, field 15)
- `mention_count` (`int32`, field 16): How many accounts the post names, and how many of them are local.
- `local_mention_count` (`int32`, field 17)

### `InboundStatus.Body`

The text of the post in one language, as sanitized HTML.

- `language` (`string`, field 1): A language tag; empty when the post does not say.
- `html` (`string`, field 2)

### `InboundStatus.Attachment`

A media attachment by its URL on the remote server. The server sends no bytes of it.

- `type` (`string`, field 1): The ActivityStreams type: Document, Image, Video, or Audio.
- `url` (`string`, field 2)
- `content_type` (`string`, field 3): The media type, empty when the post does not say.
- `description` (`string`, field 4)

### `FollowSubject`

What inbound.follow asks about: a follow from another server of an account on this one.

- `follower_uri` (`string`, field 1): The ActivityPub id of the account that follows.
- `followee_uri` (`string`, field 2): The ActivityPub id of the account on this server that it follows.

### `SignupSubject`

SignupSubject is a sign-up that is not from an invitation, once its CAPTCHA and the sign-up block
list passed it. The subject_key of the call is the ID the account would get, which identifies
this attempt alone: an applicant who tries again is a new subject.

- `email` (`string`, field 1): The email address the applicant entered.
- `ip` (`string`, field 2): The client's IP address, empty when the server does not know it.

### `LocalStatus`

A post of an account on this server, before the server creates it or stores an edit of it, for
local.status. For a new post, the subject_key of the call is the ID the post would get, which
identifies this attempt alone: an author who posts again is a new subject. For an edit, it is the
post's ID, "#", and the ID of the new revision, and the fields hold the post as edited; a reject
refuses the edit alone. Only public, local, and unlisted posts are sent, by default only those of
accounts created recently, and never those of an account whose appeal a moderator granted.

- `status_uri` (`string`, field 1): The ActivityPub id the post would get.
- `author_uri` (`string`, field 2): The ActivityPub id of the author.
- `author_username` (`string`, field 3): The author's username on this server.
- `account_created_at` (`string`, field 4): When the author's account was created, in RFC 3339.
- `visibility` (`string`, field 5): public, local, or unlisted. A local post never leaves this server.
- `in_reply_to_uri` (`string`, field 6): The post this one replies to; empty for none. The parent's text is not sent.
- `quote_uri` (`string`, field 7): The post this one quotes; empty for none. Its text is not sent.
- `language` (`string`, field 8): A language tag; "und" when the author's language is not known.
- `spoiler_text` (`string`, field 9)
- `sensitive` (`bool`, field 10): Whether the author marked the media as sensitive.
- `text` (`string`, field 11): The text as the author typed it.
- `html` (`string`, field 12): The text as the server renders it, in HTML.
- `attachment_descriptions` (repeated `string`, field 13): The alt texts of the attachments in order, empty for one without. The server sends no media.
- `poll_options` (repeated `string`, field 14)
- `hashtags` (repeated `string`, field 15)
- `mention_count` (`int32`, field 16): How many accounts the post names.

### `MediaSubject`

MediaSubject is a file a local account uploaded, once it is processed and before anyone can use
it. The subject_key of the call is the ID of the file. The original file is never sent: the
server sends the smallest image it made of the file, which has no metadata. Nothing else of the
account or of a post is sent, not even the alt text.

- `purpose` (`string`, field 1): What the file is for: attachment, avatar, header, or emoji.
- `media_type` (`string`, field 2): image, video, or audio.
- `image` (`bytes`, field 3): The smallest still image made of the file, for a video or audio file of its poster or cover.
  Empty when there is none, such as for audio without a cover.
- `image_content_type` (`string`, field 4): The media type of image, such as image/webp; empty when image is.
- `image_width` (`int32`, field 5)
- `image_height` (`int32`, field 6)
- `width` (`int32`, field 7): The size of the file as uploaded, in pixels; 0 for audio.
- `height` (`int32`, field 8)
- `duration_ms` (`int64`, field 9): How long a video or audio file plays, in milliseconds; 0 for an image.
- `animated` (`bool`, field 10): Whether the image moves: an animation, or a video. image holds its first frame or poster.

### `LocalProfile`

The profile of an account on this server, before the server stores an update of it, for
local.profile. The subject_key of the call is the account's ID, "#", and the ID of the profile
revision the update would store, so every update is its own subject; a reject refuses that
update alone, which leaves the profile as it was. The profile is public. By default only the
accounts created recently are sent. The server sends no image: only whether the update changes
the avatar or the header.

- `account_uri` (`string`, field 1): The ActivityPub id of the account.
- `username` (`string`, field 2): The account's username on this server.
- `account_created_at` (`string`, field 3): When the account was created, in RFC 3339.
- `display_name` (`string`, field 4): The display name as it would be stored.
- `note` (`string`, field 5): The bio as the user typed it.
- `note_html` (`string`, field 6): The bio as the server renders it, in HTML.
- `fields` (repeated [`LocalProfile.Field`](#localprofilefield), field 7): The profile fields in order, as they would be stored.
- `avatar_changed` (`bool`, field 8): Whether the update sets, replaces, or removes the avatar, or the header image.
- `header_changed` (`bool`, field 9)

### `LocalProfile.Field`

One profile field: its name, and its value as typed and as the server renders it in HTML.

- `name` (`string`, field 1)
- `value` (`string`, field 2)
- `value_html` (`string`, field 3)

### `CheckResponse`

- `verdict` ([`Verdict`](#verdict), field 1): A name in JSON. VERDICT_UNSPECIFIED, an empty response included, a name this version does not
  know, and a verdict the hook does not take are all failures and follow the hook's failure
  behaviour.
- `reason` ([`Reason`](#reason), field 2): A name in JSON. May be left out. A name this version does not know reads as left out.
- `note` (`string`, field 3): A short note for the moderators, in plain text. The server keeps the first 200 characters on
  one line and shows it to nobody else.

## Enums

### `Verdict`

The verdicts. Which of them a hook takes:
inbound.status: ALLOW, DROP, FLAG, MARK_SENSITIVE.
inbound.follow: ALLOW, DROP, FLAG.
signup.check: ALLOW, REQUIRE_APPROVAL, REJECT.
media.check: ALLOW, MARK_SENSITIVE, FLAG, REJECT.
local.status: ALLOW, FLAG, REJECT. A REJECT can be appealed by the author, which people review.
local.profile: ALLOW, FLAG, REJECT. A REJECT cannot be appealed; the user may change the profile and save it again.

- `VERDICT_UNSPECIFIED` = 0
- `VERDICT_ALLOW` = 1: Go on as if the hook were not there.
- `VERDICT_DROP` = 2: Stop the subject and store nothing of it.
- `VERDICT_FLAG` = 3: Go on, and file a report for the moderators.
- `VERDICT_MARK_SENSITIVE` = 4: Go on, and show the attachments as sensitive.
- `VERDICT_REQUIRE_APPROVAL` = 5: Go on, and hold the sign-up for approval.
- `VERDICT_REJECT` = 6: Refuse the subject and keep nothing of it. A person told about it sees fixed text that holds
  nothing of the response.

### `Reason`

- `REASON_UNSPECIFIED` = 0
- `REASON_SPAM` = 1
- `REASON_HARASSMENT` = 2
- `REASON_SEXUAL` = 3
- `REASON_VIOLENCE` = 4
- `REASON_ILLEGAL` = 5
- `REASON_MALWARE` = 6
- `REASON_OTHER` = 7
