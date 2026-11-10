# Following people and reading timelines

## Finding people

"Search" finds accounts: enter `@username@server` or the address of a profile, on this server or on another one. When
you are signed in, the server looks up an account it does not know yet on its own server; visitors who are not signed in
only find accounts the server already knows. Some servers do not look up accounts of other servers at all, and then
only the accounts they already know are found. The "Posts" tab searches the words of posts when the server offers it;
it finds public posts, and posts set to "This server" for people who are signed in, and only of
authors who allow it (authors can opt out in their profile settings).

On a profile of an account on another server, the page shows only the posts this server has received and links to the
original profile ("See on ...").

## Following

A profile has the button "Follow". Depending on the account it says something else:

| Button | Meaning |
| --- | --- |
| Follow | You follow at once. |
| Request to follow | The account approves its followers ("Approves followers"). The button then reads "Requested", and "Withdraw request" takes it back. |
| Follow back | The account follows you. |
| Following | You follow it. Choosing it asks "Unfollow ...?". |

When you follow an account on another server, the profile says "Waiting for ... to accept your follow request" until that
server answers. Some servers do not follow accounts of other servers; the profile then says so.

After you follow an account, the bell "Notify me of new posts" notifies you of each of its new posts.

People who want to follow your account get a request when you approve your followers: see "Follow requests" in the
settings, where you "Accept" or "Reject" each one, or "Approve" or "Decline" it from its notification. Your followers and the accounts you follow are on the profile page,
which shows how many there are.

## Timelines

| Timeline | What it shows |
| --- | --- |
| Home | Your posts, and the posts and boosts of the people you follow. It also shows posts set to "This server" by the people you follow, and the public posts and posts set to "This server" of the hashtags you follow, from people you do not follow. Posts you have not seen yet are counted ("new posts"), or inserted by themselves when you turn on "Show new posts automatically". |
| Local | The public posts, and the posts set to "This server", of the accounts of this server. |
| Federated | The public posts of the accounts of this server and of the accounts of other servers that this server knows. Posts set to "This server" and unlisted posts are not on it. |

The Local and Federated timelines are for everyone who uses the server, whether you follow them or not. A server's
operators can turn either one off, or show it only to people who are signed in; the page then says so. The settings under
"Languages of your timelines" limit the timelines to the languages you check.

A hashtag page (for example `#cats`) shows the public posts and the posts set to "This server" with that tag.

## Following hashtags

A hashtag page has the button "Follow" when you are signed in. After you follow a hashtag, your home timeline also shows
the public posts and the posts set to "This server" with it from people you do not follow, the next time you open it.
Such a post says "Followed hashtag #cats" above it, and the hashtag leads to its page, where "Following" unfollows it.
Replies come too; boosts do not, and neither do posts of people you block or mute, or of accounts and servers that
the server keeps off its public timelines. Home shows only the posts this server already has: following a hashtag does
not fetch posts from other servers.

You can follow up to 100 hashtags. "Followed hashtags" in the settings lists them, the latest first, follows another
one ("Follow a hashtag", with or without the `#`), and unfollows them.

## Featured hashtags

A profile can feature up to ten hashtags, in the order its owner chose, each with how many of the owner's public and
unlisted posts carry it and when the last one did. Choose yours under "Featured hashtags" in the profile settings:
"Add a hashtag" adds a row, the arrows move a row up or down, and "Save profile" saves them with the rest of your
profile. A hashtag you have not used yet can be featured too. Other servers that show your profile get the same list.
On a profile of another server, the counts are of the posts this server has received, so they may be lower than on
that server.

An empty home timeline says "Your home timeline is empty". Follow someone, or "Browse the local timeline"; on a server
without a local timeline, "Find people to follow" opens the search.

## Private notes

Another person's profile has a "Private note" area where you keep a note on that account, up to 2,000 characters: how
you met, what they like. Only you can see it; the account is not told, and the note does not leave this server. "Add
note" or "Edit note" opens it, and saving it empty removes the note. The archive of "Archive and deletion" in the
settings holds your notes, and a note goes when you or the other account is deleted.

## Following in a feed reader

People without an account on the fediverse can follow a profile or a hashtag in a feed reader: add the address of the
profile followed by `.atom` (for example `https://example.com/@alice.atom`), or of the hashtag page
(`https://example.com/tags/cats.atom`). Most feed readers also find the feed from the page's own address. A feed holds
the 20 newest public posts of this server's accounts, without replies or boosts; a hashtag's feed holds those with the
tag. A post with a content warning shows only the warning, and its text and media stay on the post's page.

Whether your own posts are in feeds is "Publish a feed of my public posts" under "Feeds" in the profile settings. Until
you change it, it follows "Search engines and AI": your posts are in feeds while you let search engines index your
profile. Once you change it, changing the search setting leaves it as it is. When it is off, your profile has no feed
and your posts are in no hashtag's feed. A feed reader may take a minute or two to see a change. Accounts that a
moderator has limited or suspended, and accounts that moved, have no feed.

## Lists

"Lists" puts people you follow in a list, so that you read their posts apart from your home timeline. Only you see your
lists, and nobody is told that you put them in one. You can add only the people you follow.

## Hiding what you do not want to read

Mute accounts, mute whole servers, or block accounts: see [Blocking, muting, and reporting](safety.md).
