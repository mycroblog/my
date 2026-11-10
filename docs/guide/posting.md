# Posting and choosing who sees a post

## Writing a post

Choose "Write a post" to open the form "New post". Type in "Your post" and choose "Post". The form shows how many
characters are left. Besides text you can:

- Attach images, video, or audio with "Add images, video, or audio". Describe each attachment for people who cannot see or hear it; some servers require a description for every attachment. Location and other metadata are removed from the files.
- "Add a poll", with options and a duration, and the choices "Allow choosing several options" and "Hide the votes until the poll ends".
- "Add a content warning". Readers see the warning first and choose "Show content".
- Choose "Hide the images until readers show them" to mark the media as sensitive.
- Set the language of the post, and who can reply to, boost, and quote it (see [Who can reply, boost, and quote](#who-can-reply-boost-and-quote)).
- Reply to a post ("Reply") or quote one ("Quote").

A post that could not be sent stays under "Posts being sent", where you can "Send again", "Edit", or "Discard" it.
A server can refuse a post after an automated review. When the refusal can be appealed, the post offers "Appeal".

## Who can see a post

The "Visibility" button of the form opens "Who can see this post?" for that post. The default for new posts is the
setting "Who sees your posts by default" under "Display, posting, and timelines" in the settings.

| Choice | Who can see it |
| --- | --- |
| Public | Anyone, here and on other servers. It is shown on the public timelines. |
| This server | People signed in to this server, and your followers on other servers. It is shown on this server's local timeline. |
| Unlisted | Anyone with the link, but it is not shown on the public timelines. |
| Followers | Only your followers and the people you mention. |
| Mentioned people | Only the people you mention. |

![Who can see a post on this server and on other servers, for each of the five visibilities. The list below says the same.](images/visibility.svg)

Row by row, on this server and then on other servers:

- **Public**: anyone, also people who are not signed in, and the post is shown on the local and federated timelines. On other servers anyone can read it, and it is sent to your followers and the people you mention.
- **This server**: the people signed in to this server, and the post is shown on the local timeline. Other servers receive it only for your followers there, who see it as a "Followers" post; it is not sent to anyone else.
- **Unlisted**: anyone with the link, but the post is not shown on the public timelines. On other servers too, anyone with the link can read it, and it is sent to your followers and the people you mention.
- **Followers**: your followers and the people you mention, on this server and on other servers.
- **Mentioned people**: only the people you mention, on this server and on other servers.

On a server of one person, where nobody else is signed in, "This server" means no more than "Followers": new posts
cannot have it, and a default of "This server" works as "Followers". The visibility menu and the default in "Display,
posting, and timelines" leave it out, and a reply to a "This server" post is for "Followers". Posts that already have it
keep it.

A block always wins: someone who blocks you, or whom you block, does not see your post whatever you chose.

### "This server"

A post set to "This server" stays on your server. People who are not signed in to it cannot read it, even with the
link, and it never appears on the federated timeline. Other servers receive it only for your followers there, and they
show it to those followers as a post for followers only.

- If you mention someone on another server who does not follow you, they do not receive the post, and your followers there see the mention as a plain link. The form warns you about this.
- Accounts of your server can boost a "This server" post, and the boost is shown only on this server.
- A reply to a "This server" post starts as "This server" too; you can change it.

### Other things to know

- You cannot change the visibility of a post after posting it. When you edit a post, the visibility and the post it replies to stay as they are.
- A moderator can limit how widely an account posts. The form then says "Your account can post at most as ..." and a wider choice is posted at the widest level allowed.
- Anyone who can see a public, "This server", or unlisted post can boost it. Only you can boost your own "Followers" posts, and only your followers see that boost. "Mentioned people" posts cannot be boosted.
- The author of a post can choose who may reply to it and who may boost it; posts from some other servers carry such a choice too. A reply or a boost outside it is refused, with a message such as "The author of this post does not let you reply to it." The author, the people the post mentions, and the author of the post it replies to can always reply.
- The author may also let some people reply or boost only once they approve it. Such a reply or boost waits: you are notified ("... asked to reply to your post"), and it stays off your post until you approve it. A waiting reply from another server is not in the conversation under your post or in its count of replies; it still appears on its author's profile and on its own page. A waiting boost is not counted. Approving it adds it to your post; declining it keeps it off, and the other server is told either way.
- A reply or a boost from another server that is outside your choice altogether is kept off your post in the same way, and notifies you of nothing.
- When you reply to or boost a post whose author approves replies or boosts by hand, yours waits for that approval. Until then, only you see your reply, under the post and on its own page, marked as waiting; others see nothing of it, and nothing is sent to other servers. Your boost is not made until it is approved. You are notified when the author approves or declines it ("... approved your reply, boost, or quote").
- You can pin up to five of your posts to your profile with "Pin to profile". A "Mentioned people" post cannot be pinned.

## Who can reply, boost, and quote

The button "Who can reply, boost, and quote" among the tools of the form opens the choices for that post. For each of
replies, boosts, and quotes, choose who can do it at once: "Anyone", "Followers and people you mention", "People you
mention", or "Only you". Below each, "Others can reply with your approval" (and the same for boosts and quotes) lets
everyone else ask instead of being refused. Once a choice admits less than anyone, the form shows the result above its
tools, for example "Replies: Anyone. Boosts: Only you. Quotes: Anyone.", before you post.

- The people a post mentions can always reply to it. A "Followers" post can be boosted and quoted only by you, and a "Mentioned people" post by no one, whatever you choose.
- The defaults for new posts are under "Who can reply to, boost, and quote your new posts" in "Display, posting, and timelines" in the settings.
- To change them for a post you already sent, choose "Change who can reply, boost, and quote" in its "More actions" menu. Each choice is saved at once. Replies, boosts, and quotes you approved before stay, and requests that the new choice admits are approved.
- Requests that wait for you are listed under "Approval requests" in the settings, next to "Follow requests"; the notification of each one links there with "Review the requests". "Approve" adds the reply, boost, or quote to your post, and the conversation under it shows an approved reply at once; "Decline" keeps it off.
- On someone else's post, "Reply" is not shown and "Boost" cannot be pressed when its author does not let you. When the author approves them by hand, the form says so before you reply, and after you boost, the post says "Your boost waits for the author's approval."

## After posting

Under a post you find "Reply", "Boost", "Quote", "Favourite", and "Bookmark". The menu "More actions" has "Copy link",
and for your own posts "Edit", "Pin to profile", and "Delete". A post that was edited shows "Edited", and its earlier
versions are on its "Edit history" page. "Delete" removes the post for everyone, along with its boosts and favourites.

For a post by another person, the same menu offers to follow, mute, block, or report its author, and "Direct message".
Your bookmarks are listed on the "Bookmarks" page; only you see them.

When the server offers translation, or your browser can translate, posts in other languages have "Translate" among
their actions, and in the "More actions" menu, which holds it when the post is too narrow for one more button. While a
post shows its translation, a line under the text says which language it was translated from and by whom, with "Show
original" to go back. If the server sends the text of posts to a translation service, the visibility picker tells you
for which choices.

## Direct messages

"Direct messages" in the navigation gathers your "Mentioned people" posts and those you receive, one row per
conversation, the one with the newest post first. Each row shows who is in the conversation, when its newest post was
written, and the start of that post. The number beside "Direct messages" is how many conversations have a post you
have not read yet; each of them has a dot before its names.

Choose the start of a post to open the conversation's thread; this marks the conversation read on all your devices. A
reply to a "Mentioned people" post is always "Mentioned people" too, so it reaches no one the conversation did not. On
phones, "Direct messages" is in "Menu" unless you move it into the tab bar under "Display, posting, and timelines" in
the settings.

## Deleting old posts automatically

"Delete old posts" in the settings deletes your posts once they are older than the age you choose, as if you had
deleted them yourself: they go here and on the servers they were sent to, and cannot be restored. The posts you keep
(pinned, bookmarked, or favourited by you, with media or a poll, of some visibilities, or with many boosts or
favourites) stay. Your boosts of other people's posts are never deleted. The server checks every hour and deletes at
most 100 of your posts each time, so many old posts take several hours to go.

A new account starts with the settings the server chose for new accounts; they are yours from then on, and you can
change them like any other setting.

A server can also keep posts for at most some days. The server's "About" page says so before you sign up. Under such a
maximum, automatic deletion is always on for you, the age you choose cannot be longer than the maximum, and only the
kinds of posts that the server allows can be kept (by default only pinned posts). Some servers let some people keep
their posts longer, for example as a benefit of a supporter plan.

When a maximum starts to apply to you (the server introduces or lowers it, or you lose the right to keep posts longer),
you get a notification, and an email unless you turned that off, saying from when it applies. Until then your own
settings stay in effect. The settings page shows the date and, with its preview, which of your posts will be deleted then.
