# 用語集

| 用語 | 意味 |
| --- | --- |
| インスタンス | 本ソフトウェアの 1 デプロイ。1 つのドメインを持つ |
| ローカル / リモート | 自インスタンスに属する / 他インスタンスに属するアカウント・投稿 |
| Actor | ActivityPub のアクター。本システムでは主にアカウント（`Person` / `Service`）と、インスタンス自身（`Application`） |
| Activity | `Create` `Follow` などの ActivityPub アクティビティ |
| 配送 (delivery) | ローカルで発生した Activity をリモートの inbox に POST すること |
| 受信 (inbox processing) | リモートから受け取った Activity を検証・反映すること |
| ポート (port) | ドメイン/アプリケーション層が外部資源に求めるインターフェイス |
| アダプタ (adapter) | ポートの具体実装（例: `queue/cloudtasks`, `db/sqlite`） |
| ターゲット (target) | 実行環境の組み合わせ（GCP / AWS / Self-hosted） |
| エントリポイント (entrypoint) | ターゲットごとの起動コード（HTTP ハンドラ・キューコンシューマ・cron のバインド） |
| コントラクトテスト | 1 つのポートに対し、全アダプタが共通で通すべきテストスイート |
| Visibility | 投稿の公開範囲: `public` / `local` / `unlisted` / `private` / `direct`。`local` はローカルタイムラインに載るが外部へはフォロワーにだけ届く |
| double-knocking | 署名方式を RFC 9421 で試し、失敗時に draft-cavage で再試行する手法 |
| ADR | Architecture Decision Record。`docs/adr/` に置く設計判断の記録 |
