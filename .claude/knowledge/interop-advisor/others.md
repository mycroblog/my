# Bonfire, Activity-Relay, Fedibird, and shared behaviour

## Bonfire Social

Base: <https://github.com/bonfire-networks/bonfire-app> (1.0.7, live 2026-11-10: TestInteractionApprovalBonfire).

- Quotes round-trip both ways with `QuoteRequest`; its quote gets `quoteAuthorization` on my approval and becomes approved here with the following `Update`
- Its pending quote post has no `quote` until approved -> my answers a QuoteRequest even when the quoting post has no `quote` yet
- Sometimes embeds my `Create` instead of the quoted post as the `QuoteRequest`'s `object` -> my reads that object as the quoted post
- Sends and answers no reply/boost requests; reads `automaticApproval` only, not `manualApproval`, so it refuses at once a reply to a post that auto-approves only its author [ADR-0155 §6]
- Dropped my `Accept` of a quote of a post it learned by `Create` as a follower ("Could not find local quote post or quoted object for Accept", `Bonfire.Social.Quotes.ap_receive_activity`, quotes.ex:979); a quote of a fetched `Note` works -> the live test unfollows before quoting (cause unconfirmed)
- Records a received `QuoteRequest` only after its `RemoteFetcherWorker` has created the quoting post (`instrument` looked up by `AdapterUtils.return_pointable`); a request processed earlier is dropped unanswered, embedding `instrument` does not help

## Activity-Relay (Mastodon-mode relays)

- Relays the original activity unchanged; Mastodon and Misskey subscribers drop it unless LD-signed (Mastodon also takes an FEP-8b32 proof) -> my LD-signs public Create/Update/Delete for every recipient [ADR-0006 History, source read 2026-10-20; live: TestRelay]
- Takes the `Application` instance actor as a LitePub-style follower; its Mastodon-mode path does not handle `Announce` -> my never sends boosts to relays [ADR-0006 §18, §19]

## Fedibird

- Quotes with `quoteUri` -> my reads it and sends it alongside `quote` [TestNormalizeDialects]

## Shared by several servers

- Some servers answer 404 to the instance actor's fetch of a followers-only post -> an operator refetch deletes on 410 or Tombstone only, never on 404 [ft: RefetchesStoredPostsOnRequest]
- Mastodon, Misskey, Akkoma, GoToSocial all count the poll-vote `Note` shape as a vote [mastodon-content.md]
