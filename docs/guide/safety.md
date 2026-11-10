# Blocking, muting, and reporting

You decide what you read and who can reach you. Blocking, muting, and keyword filters are yours alone; a report goes to
the moderators of the server.

## Blocking an account

Open the menu "More actions" on the profile of the account (or on one of its posts) and choose "Block". The client asks
"Block ...?" and says what follows: you stop following each other, and neither of you can follow the other until you
unblock them. The follows that the block ended do not come back when you unblock.

A block also hides the posts of the two of you from each other.

"Blocked accounts", under "Relationships" in the settings, lists the accounts you block and lets you "Unblock" them.

## Muting an account

"Mute" in the same menu hides the posts of the account, and its notifications, from you. It does not change who follows
whom. "Muted accounts", under "Relationships" in the settings, lists them, shows when a mute ends ("Until ...") and
whether "Notifications hidden too", and lets you "Unmute".

## Muting a server

"Muted domains", under "Relationships" in the settings, mutes a whole server (for example `mastodon.example`; its
subdomains are muted too). It hides the posts of its accounts from your timelines other than home, their boosts from
your home timeline, and their notifications. The accounts of that server you follow stay on your home timeline. Only you
are affected, and the server is not told.

## Filtering posts by keyword

A keyword filter folds or hides the posts that contain one of its words, whoever wrote them. The server matches them, so
every app you use shows the same result. A filter has a title, its words, where it applies, what it does, and optionally
when it ends:

- **Where it applies:** your home timeline and lists, your notifications, the local, federated, and hashtag timelines,
  threads, and profiles. Each filter applies only where you choose. Searching, your bookmarks and favourites, and your
  direct messages are never filtered.
- **What it does:** "warn" folds a post behind the title of the filter, with a way to show it; "hide" leaves the post
  out, and its notification too. A post you open yourself is folded rather than hidden, so a link to it still shows
  something.
- **When it ends:** a filter with an end stops applying at that moment. It stays in your list for 30 days, marked as
  expired, so that you can move its end later and use it again; after that it is deleted.

A word matches the text of a post, its content warning (and the text behind it), the choices of its poll, and the
descriptions of its images. Upper and lower case, half-width and full-width forms, and katakana and hiragana all match
each other, so `ネタバレ` also matches `ねたばれ` and `ﾈﾀﾊﾞﾚ`. A phrase with spaces matches those words in that order.
Links match by the text they show, not by the address behind them.

A word matches anywhere it appears, so "cat" also matches "concatenate". Choosing "whole word" for a word makes it match
only where no letter or digit touches it. Languages written without spaces between words, such as Japanese, Chinese,
and Thai, have nothing to tell where a word ends, so there a word always matches anywhere: `猫` also matches `子猫`.
Choose longer words in those languages to avoid matching more than you mean.

Your own posts and boosts, and the posts they quote, are never filtered. When only a quoted post matches, only the quote
is folded, even for "hide". A boost is filtered by the post it boosts.

A filter applies at once to every post, old ones too, but an app shows the change when it loads the timeline again.
You can have up to 100 filters, with up to 100 words each and 1,000 words in all.

## Reporting an account or a post

Report what breaks the rules of the server, or the law. From the menu "More actions" on a post or a profile, choose
"Report". A dialog leads you through the steps:

1. **Why are you reporting this account?** Choose "Spam", "Illegal content", "Breaks the server rules" (offered when the server has published rules), or "Something else".
2. **Which rules does it break?** Only for "Breaks the server rules": choose every rule that applies. The rules are the ones the server's operators published.
3. **Which posts are you reporting?** Choose the posts the moderators should look at. You can also report the account without posts. When you start from a post, it is already chosen.
4. **Anything else the moderators should know?** An optional comment that only the moderators read.

For an account on another server, the last step also offers "Also send the report to ...". The moderators of that server
then get a copy without your name. Choose "Send report" to finish.

The moderators of your server review the report. The account is not told who reported it. When the report is resolved,
you get a notification ("A report you filed has been resolved").

A voice chat room can be reported too, from its page ("Report room"); the report is about the room and its host.

## When a moderator acts on your account

The moderators of a server can limit or suspend an account. "Account status", under "Account and data" in the settings,
lists the actions in effect ("Actions in effect") with their reason and when each one ends, and lets you appeal an
action once ("Appeal"). You are notified of the moderators' notices and of the result of an appeal.

What the moderators may do, and which rules apply, is up to each server: read its rules before you report or appeal.
