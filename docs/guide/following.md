# Following people and reading timelines

## Finding people

"Search" finds people by name or handle, and hashtags; for another server's account, enter `@username@server` or the
address of its profile. When you are signed in, the server looks up an account it does not know yet on its own server;
visitors who are not signed in only find accounts the server already knows. Some servers do not look up accounts of other servers at all, and then
only the accounts they already know are found. The "Posts" tab searches the words of posts when the server offers it;
it finds public posts, and posts set to "Local" for people who are signed in, and only of
authors who allow it (authors can opt out in their profile settings).

On a profile of an account on another server, the page shows only the posts this server has received and links to the
original profile ("See on ...").

## Following

A profile has the button "Follow". Depending on the account it says something else:

| Button | Meaning |
| --- | --- |
| Follow | You follow at once. |
| Request to follow | The account approves its followers ("Follow requests need approval"). The button then reads "Requested", and "Withdraw request" takes it back. |
| Follow back | The account follows you. |
| Following | You follow it. Choosing it asks "Unfollow ...?". |

When you follow an account on another server, the profile says "Waiting for ... to accept your follow request" until that
server answers. Some servers do not follow accounts of other servers; the profile then says so.

On another server's page, the buttons that follow an account or reply to a post can bring you to this server: enter your
handle (`@username@server`) where that page asks for it. This server then opens "Open from another server", after you
sign in, with the address of the account or the post. "Show the account" opens its profile here, where "Follow"
follows it; "Reply to the post" opens the post here with a reply to it started. Nothing is looked up until you press the
button.

After you follow an account, the bell "Notify me of new posts" notifies you of each of its new posts.

When you approve your followers, people who want to follow you send a request. The row "N follow requests" above your
notifications opens them, and you "Approve" or "Decline" each one. Your followers and the accounts you follow are on the
profile page, which shows how many there are.

"Who can see your following and followers lists" under "Profile" in the settings decides who can open those two
lists: "Anyone" (the default), "Followers" (people on this server whose follow you accepted), or "Only you". You
always see both lists, and everyone still sees how many there are. Other servers get only the numbers unless you
choose "Anyone". A list you cannot open says so in place of the accounts.

## Timelines

| Timeline | What it shows |
| --- | --- |
| Home | Your posts, and the posts and boosts of the people you follow. It also shows posts set to "Local" by the people you follow, and the public posts and posts set to "Local" of the hashtags you follow, from people you do not follow. New posts show at the top as they arrive. While you read further down, the "new posts" button over the timeline counts the ones that came in above you. If you turn off "Show new posts as they arrive" (under "Timelines" in the "Display and posting" settings), new posts wait until you press that button. |
| Local | The public posts, and the posts set to "Local", of the accounts of this server. |
| Federated | The public posts of the accounts of this server and of the accounts of other servers that this server knows. "Local" and "Quiet public" posts are not on it. |

The Local and Federated timelines are for everyone who uses the server, whether you follow them or not. A server's
operators can turn either one off, or show it only to people who are signed in; the page then says so.
"Languages to show", under "Timelines" in the "Display and posting" settings, limits every timeline to the languages
you choose; posts with no language set still show.

A hashtag page (for example `#cats`) shows the public posts and the posts set to "Local" with that tag.

## Following hashtags

A hashtag page has the button "Follow" when you are signed in. After you follow a hashtag, your home timeline also shows
the public posts and the posts set to "Local" with it from people you do not follow, the next time you open it.
Such a post says "Followed hashtag #cats" above it, and the hashtag leads to its page, where "Following" unfollows it.
Replies come too; boosts do not, and neither do posts of people you block or mute, or of accounts and servers that
the server keeps off its public timelines. Home shows only the posts this server already has: following a hashtag does
not fetch posts from other servers.

You can follow up to 100 hashtags. "Followed hashtags", under "Relationships" in the settings, lists them, the latest
first, and lets you "Unfollow" them.

## Featured hashtags

A profile can feature up to ten hashtags, in the order its owner chose, each with how many of the owner's "Public" and
"Quiet public" posts carry it and when the last one did. Choose yours in "Featured hashtags" under "Profile" in the
settings: type a hashtag and press Enter to add it after the others, and press the × on one to remove it; each change is
saved at once. A hashtag you have not used yet can be featured too. Other servers that show your profile get the same list.
On a profile of another server, the counts are of the posts this server has received, so they may be lower than on
that server.

A featured hashtag opens that person's posts with the hashtag, newest first, that you may see on their profile
("Posts of Alice with #cats"); boosts are not listed. "All posts with #cats" opens the page of the hashtag with
everyone's posts.

While you follow no one, your home timeline shows people the server suggests; "Search" shows them before you type.

## Private notes

On another person's profile, "Add a note only you can see" keeps a note on that account, up to 2,000 characters: how
you met, what they like. The account is not told, and the note does not leave this server. Press the note to edit it;
saving it empty removes it. The archive of "Archive and deletion" in the settings holds your notes, and a note goes
when you or the other account is deleted.

## Following in a feed reader

People without an account on the fediverse can follow a profile or a hashtag in a feed reader: add the address of the
profile followed by `.atom` (for example `https://example.com/@alice.atom`), or of the hashtag page
(`https://example.com/tags/cats.atom`). Most feed readers also find the feed from the page's own address. A feed holds
the 20 newest public posts of this server's accounts, without replies or boosts; a hashtag's feed holds those with the
tag. A post with a content warning shows only the warning, and its text and media stay on the post's page. An article
shows its title and summary, with a link to the article. To follow only the articles of a profile, add `/articles.atom`
to the profile's address instead (`https://example.com/@alice/articles.atom`); it follows the same rules.

Whether your own posts are in feeds is "Offer an Atom feed of your public posts" under "Profile" in the settings. Until
you change it, it follows "Include your profile and public posts in search": your posts are in feeds while that is
on. Once you change it, changing the search setting leaves it as it is. When it is off, your profile has no feed
and your posts are in no hashtag's feed. A feed reader may take a minute or two to see a change. Accounts that a
moderator has limited or suspended, and accounts that moved, have no feed.

## Lists

"Lists" puts people you follow in a list, so that you read their posts apart from your home timeline. Only you see your
lists, and nobody is told that you put them in one. You can add only the people you follow.

## Hiding what you do not want to read

Mute accounts, mute whole servers, or block accounts: see [Blocking and muting](safety.md).
