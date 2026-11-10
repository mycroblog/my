# Keyword filters

A keyword filter folds or hides the posts that contain one of its words, whoever wrote them: spoilers of a show you
have not seen yet, a topic you need a break from. The server matches the posts, so every app you use shows the same
result. Only you see your filters, and nobody is told about them.

## Adding a filter

Open "Keyword filters" under "Display and notifications" in the settings, and choose "Add a filter":

1. **Name:** what a folded post shows in its place ("Filtered: Season finale"), up to 50 characters.
2. **Words:** one or more words or phrases. "Add a word" adds another, up to 100 in a filter. "Whole word" beside a word
   makes it match only as a whole word (see [How words match](#how-words-match)).
3. **Where it applies:** your home timeline and lists, your notifications, the local, federated, and hashtag
   timelines, threads, and profiles. A new filter applies everywhere; clear the places you do not want.
4. **What it does:** "Fold" puts the post behind the filter's name, with "Show" to read it. "Hide" leaves the post
   out, and its notification too.
5. **Ends:** "Never", or after one of the durations offered, from 30 minutes to a week.

Choose "Add". "Edit" on a filter changes any of these, and "Delete" removes it with its words after asking. A filter
applies at once to every post, old ones too; the timelines open on screen load again to show the change.

Searching, your bookmarks and favourites, and your direct messages are never filtered. Neither are your own posts and
boosts, nor the posts they quote.

## What a filtered post looks like

A post that a "Fold" filter matches shows only "Filtered:" and the names of the filters it matched, and "Show". Its
author, text, and pictures stay out of sight, and a screen reader reads only that line, until you choose "Show". A post
folds again when you come back to the page.

A notification about such a post shows the same line instead of the post's text. When only a quoted post matches, only
the quote is folded, even for "Hide", with "Show the quoted post". A boost is filtered by the post it boosts.

A post you open yourself, from a link or a notification, is folded rather than hidden, so the link still shows
something.

## When a filter ends

A filter with an end stops applying at that moment. It stays in your list, marked "Expired", until you delete it. To use
it again, choose "Edit" and a new end, or "Never". While you edit a filter that has an end, "Keep" leaves the end as it
is.

## How words match

A word matches the text of a post, its content warning (and the text behind it), the choices of its poll, and the
descriptions of its images. Links match by the text they show, not by the address behind them. A phrase with spaces
matches those words in that order; any run of spaces or line breaks in a post matches one space.

Upper and lower case, half-width and full-width forms, and katakana and hiragana all match each other:

| Word | Also matches |
| --- | --- |
| `Finale` | `finale`, `FINALE`, `ｆｉｎａｌｅ` |
| `ネタバレ` | `ねたばれ`, `ﾈﾀﾊﾞﾚ` |
| `season finale` | `Season  Finale`, and the two words across a line break |

A word matches anywhere it appears, even inside a longer word. "Whole word" makes it match only where no letter, digit,
or `_` touches it on either side:

| Word | Without "Whole word" | With "Whole word" |
| --- | --- | --- |
| `cat` | `cat`, `cats`, `concatenate` | `cat`, `cat!`, `#cat`, but not `cats` |
| `vote` | `vote`, `voters`, `devoted` | `vote`, `Vote.`, but not `voters` |

Some scripts write words without spaces between them: Chinese, Japanese (kanji, hiragana, and katakana), Thai, Lao,
Khmer, and Myanmar, among others. A filter cannot tell where a word ends in them, so their letters never count as
touching, and "Whole word" works as if it were off: `猫` also matches `子猫` and `猫舌`, and `選挙` also matches
`選挙速報`. Choose longer words in those languages to avoid matching more than you mean, for example `最終回のネタバレ`
rather than `最終回`. Next to a letter of another script, "Whole word" still applies: `AI` with "Whole word" matches
`AIの話` but not `AIDS`.

A filter has no wildcards or regular expressions. Its words match when any one of them does; to fold posts only when two
words come together, write them as one phrase.

## Limits

You can have up to 100 filters, with up to 100 words each and 1,000 words in all. A word is up to 100 characters.

The archive of "Archive and deletion" in the settings holds your filters, expired ones included, as
`keyword_filters.json`.
