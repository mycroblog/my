# Full-text search of posts

A guide for operators: what the full-text search of posts holds, how to turn it on and change it, how to rebuild its index, and what it asks of your logs and backups.

## Nothing to enable yet

No search engine is supported yet: the only value of [`search`](../reference/configuration.md#search) is `none`, the default, and the server runs without a full-text search of posts. With `none` the server connects to nothing, queues no search job, and the client shows no way to search posts. The search of accounts and hashtags works from the database and does not depend on this setting.

The commands and steps below apply once a search engine is available. On a server with `search = none`, each `my search` command says so and does nothing.

## What the index holds

The database stays the source of truth, and the index is a copy you can throw away and rebuild. An engine returns post IDs only: the server reads each post again from the database and applies the usual visibility rules, so an index that lags behind can leave a post unfound but cannot show one that should not be seen.

A post is in the index when all of these hold:

- It is not deleted.
- Its visibility is `public` or `local`, and [`search_visibilities`](instance-settings.md) includes it. `unlisted`, `private`, and `direct` posts are never indexed, whatever the setting.
- Its author has turned on `indexable`, the setting described in [Search engines and AI crawlers](crawlers.md).
- Its author, and the author's domain, have no restriction that hides them from discovery.

The index holds the body, the content warning, and the alternative texts of the current version of each post, with its author and language. It does not hold polls, profiles, or older versions.

`indexable_default` is `false` by default, so a server that has just turned search on finds little until people choose `indexable` themselves.

## Turning it on

1. Run the engine, and set `search` and the settings of its backend.
2. If you run workers separately, decide on [their layout](#keeping-search-from-delaying-other-jobs).
3. Configure [your logs](#the-query-appears-in-urls).
4. If the engine is run by someone else, add it to your [privacy documents](#your-privacy-documents).
5. Restart the server, and from then on new posts go into the index.
6. Run `my search rebuild` to add the existing posts.

Search works during the rebuild, and its results are incomplete until it ends.

## Rebuilding the index

```sh
my search rebuild              # empty the index, then add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second
my search status               # the progress and the failures
my search clear                # empty the index and end any rebuild
```

The command returns once the rebuild is under way, and the worker does the rest as `search.rebuild` jobs ([jobs reference](../reference/jobs.md#full-text-search)), so keep a worker running. Each job covers 100 posts.

- **Rate.** The default is 20 posts a second, at most 1000, and it counts every `public` and `local` post looked at, including those that do not qualify. At the default, a million posts take about 14 hours. It leaves room for other jobs and the database, so raise it only if your server has room to spare.
- **Which one to run.** The default removes everything that should not be in the index: documents left by a deletion that never arrived, or by restoring the database from a backup. Search finds less until it ends. Use `--keep` after [adding a visibility](#changing-search_visibilities), when nothing needs removing and search should stay complete. `--keep` never removes a document of a post that is no longer in the database.
- **Interrupted?** Run the same command again. It goes on from the last post handled and does not empty the index again. Add `--rate` to change the rate. A rebuild also goes on by itself when a worker restarts.
- **The other mode.** While a rebuild is unfinished, the command in the other mode is refused, because it would not do what it says. To start over in either mode, run `my search clear` and then the command you want.
- **Running it twice** gives the same index. Posts created, edited, or deleted during a rebuild end up right, because their own jobs keep running and every job reads the post as it is at that moment.

`my search status` shows the backend, `search_visibilities`, and for the latest rebuild its phase (`clearing`, `filling`, or `finished`), the posts handled and failed, the rate, and the last error.

When something goes wrong:

- **The engine is down.** If three posts in a row fail, the job stops where it is, records the error, and retries. A rebuild that ran out of retries leaves a failed job (`my job list`), and `updated` in the status stops moving. Once the engine is back, run the same command to go on.
- **The engine refuses a post.** The post is recorded as failed and the rebuild moves on. The status lists up to 20 of them. Fix the cause, then run `my search rebuild --keep` to try every post again.

## Changing `search_visibilities`

Leaving a visibility out removes its posts from the index by itself. Adding one finds only new posts, until you run `my search rebuild --keep`.

**Tell your users before you add `local`.** `indexable` then covers the `local` posts of everyone who already turned it on, and the rebuild adds their earlier `local` posts, without asking them again. The profile settings say what `indexable` covers on your server, but people who chose it long ago will not see that unless you tell them.

## Switching off or changing the engine

When the server moves to `search = none` or to another engine, it loses its connection to the old index. That index is not cleaned any more, so posts deleted afterwards stay in it, text included.

1. Stop the server and its workers, so that nothing writes to the index after the next step.
2. Run `my search clear`.
3. Change `search`, and start the server.
4. With a new engine, run `my search rebuild`.

If you already switched, or cannot stop the server, delete the old index in the engine itself. `my search clear` cannot reach it once the setting has changed. Jobs queued before the change do nothing after a move to `none`.

**Coming back from `none`.** Run `my search rebuild` without `--keep`: the server keeps no record of what was deleted while it had no engine, and only emptying the index removes those posts. If a rebuild was unfinished when you left, run `my search clear` first, because a resumption does not empty the index.

## Keeping search from delaying other jobs

By default a worker runs one job at a time, so a slow engine shares that worker's time with `federation.inbox`, even though each call of the engine has a 10-second limit. A busy server should separate them:

- Run one worker process with `worker_job_types = search.*` and the others with `worker_job_types_except = search.*` ([Splitting workers by job type](running.md#splitting-workers-by-job-type)).
- With `queue = cloudtasks`, send the `search.*` types to their own queue with a [queue route](running.md#routing-job-types-to-their-own-queues).

A small server can leave them together, and search jobs then share the time with federation.

## The query appears in URLs

`GET /api/v1/statuses/search` carries what people search for in its URL, as `?q=`. The server's own `http request` log line holds the path and the route, not the query string, and traces and error reports leave it out too. Whatever sits in front of the server may record it, though, and a search can say a lot about a person. Check each layer of your setup, and either keep the query out of its log or treat that log as personal data, with access and retention to match.

| Layer | Does it record the query? | What to do |
| --- | --- | --- |
| Cloud Run | Yes. Its request logs are created automatically, and `httpRequest.requestUrl` includes the query. | Exclude them, as below. |
| Google Cloud load balancer | Only if you turn on logging for the backend service. It is off by default on new ones, and uses the same field. | Leave it off, or exclude them, as below. |
| AWS Application Load Balancer | Only if you turn on access logs, which are off by default. The request line holds the URL as the client sent it. It has no way to drop part of a line. | Leave them off, or give them a short lifecycle rule and limited access. |
| CloudFront | Only if you turn on standard logging, in the field `cs-uri-query`. | With standard logging v2, leave that field out when you select the fields. |
| ECS | No request log of its own: its logs hold what the server writes. | Nothing. |
| Cloudflare | Only if you use Logpush, in the field `ClientRequestURI`. | Select `ClientRequestPath` instead. |
| Caddy | Only with a `log` directive, which `compose.caddy.yaml` does not have. | If you add one, remove the parameter, as below. |
| Other reverse proxies | Usually yes: their default formats log the whole request line. | Log the path without the query for this route, or turn off its access log. |

On Google Cloud, this keeps both logs out of the `_Default` bucket of the project. Add the same exclusion to any sink of your own:

```sh
gcloud logging sinks update _Default \
  --add-exclusion='name=my-search-queries,filter=httpRequest.requestUrl:"/api/v1/statuses/search"'
```

In Caddy:

```caddyfile
social.example {
  log {
    format filter {
      request>uri query {
        delete q
      }
    }
  }
  reverse_proxy my:8080
}
```

## Backups

The index holds the text of posts, so it needs the same care as the database: who can reach it, where it is kept, and how long.

A snapshot of the engine is the part to watch. A post deleted from the index stays in every snapshot taken before, just as it does in database backups, so give snapshots the same retention as your [database backups](backup.md).

You do not need snapshots to recover: `my search rebuild` makes the index again from the database. After restoring the database from a backup, run it without `--keep`, so that posts newer than the backup leave the index.

## Your privacy documents

If the engine runs on a service of another company, that company receives the text of posts covered by `search_visibilities`, including `local` posts if you include them. Name it as a recipient in the [privacy policy](privacy-policy-template.md) and the [record of processing activities](processing-record-template.md). An engine you run yourself is no new recipient.

Also say what you decided about [logs that hold the query](#the-query-appears-in-urls), if you keep them.

## Closing the service

The server does not delete data itself when it closes, so discard the index and the engine's snapshots along with the database and the media ([Closing the service](service-closure.md#7-discarding-the-db-blobs-and-backups)).
