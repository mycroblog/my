# 連合の設計（ActivityPub S2S）

> 決定は [ADR-0006](../adr/0006-federation-protocol-scope.md)（範囲・署名・受信・取り込み・リレー・連合の向き）と、[ADR-0017](../adr/0017-visibility-local.md)・[ADR-0018](../adr/0018-federated-uris.md)・[ADR-0019](../adr/0019-security-baseline.md)・[ADR-0058](../adr/0058-moderation-admin-separation.md)・[ADR-0088](../adr/0088-service-shutdown-modes.md)・[ADR-0089](../adr/0089-account-migration-move.md) にあります。振る舞いは `pkg/testkit/federationtest` のスイートが、ほかのサーバーの振る舞いはエージェント `interop-advisor` の知識（`.claude/knowledge/interop-advisor/`）が持ちます。

## 1. モデレーションとの連携

`federation` は `moderation` の投影を、公開 API（`moderation.FederationPolicy`）からだけ読みます。処分が連合の入口ごとに止めるものは次のとおりです。判定はサブドメインにも効きます。

| 制限 | 受信 | 送信 | 取得・表示 |
| --- | --- | --- | --- |
| ドメインの `federation`（`suspend`） | 署名の検証より前に `403` | 送らない | 取得しない。既存の投稿・アカウントは `profile_view` で隠す |
| ドメインの `public_discovery` / `unsolicited_reach`（`limit`） | 受け付ける | 送る | 連合タイムライン・検索に出さない。フォローしていない相手への通知を作らない |
| ドメインの `media_fetch`（`reject_media`） | 受け付ける | 送る | メディアプロキシが取得しない |
| ドメインの `report_intake`（`reject_reports`） | `Flag` を捨てる | — | — |
| ドメインの `media_sensitive`（`force_sensitive`） | 受け付ける | 送る | 添付をセンシティブとして表示 |
| アカウントの `federation`（`suspend` / `ban`） | リモートなら `Delete` / `Undo` / `Reject` 以外を捨てる | ローカルなら、処分の伝播（ADR-0058 §6）のアクティビティ以外を送らない | ローカルの Actor の GET は、空のプロフィールに `suspended: true` を付けた Actor を返す（鍵は返す）。投稿などのオブジェクトの GET は `404` |
