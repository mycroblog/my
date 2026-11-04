# ADR-0002: サーバーの言語と構造: Go・モジュラーモノリス・Ports & Adapters

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #6, #2

## Decision（決定）

### 1. モジュラーモノリス + Ports & Adapters

- 層: `internal/domain`（純粋）→ `internal/app`（ユースケース・ポート定義）→ `internal/interface`（Client API / ActivityPub / Admin）/ `internal/adapter`（ポート実装）→ `internal/platform` と `cmd/`（ターゲット別の組み立て）。
- モジュール（`identity` `account` `content` `graph` `timeline` `notification` `federation` `moderation` `instance` `diagnostics` `webhook` `finance` `supporters` `voice`。`internal/domain` に同じ名前のパッケージを持つ）は
  アプリケーション層の公開 API 経由でのみ相互参照する。DB テーブルの所有者はモジュール単位で決める。
- 依存性注入はコンストラクタ / ファクトリ関数で明示的に行い、DI コンテナは使わない。組み立ては `internal/platform` と `cmd/` の 1 箇所（composition root）。
- 依存方向は lint（golangci-lint の depguard）で CI 検査する。
- モジュール間の非同期連携はドメインイベント → Transactional Outbox → JobQueue。
- ポートは「2 つ以上の実装が現に存在する or 確実に必要」なものに限る。
- `internal/app` には、モジュールのほかに共通の部品（`kernel` `queue` `uow` `blob` `media` `metrics` `projection` `stream` `dbupgrade` `rewrap`）も置く。ドメインを持たない。

### 2. サーバーは Go で書き、Cloudflare Workers は対象外とする

- **サーバーは Go で実装する。Cloudflare Workers は実行環境の選択肢から除外する。**
- 対応ターゲットは **Google Cloud / AWS / Self-hosted（Docker, microk8s）の 3 系統** とする。Cloudflare Workers・D1・Cloudflare Queues 向けのアダプタとエントリポイントは v0 では作らない。
- 連合の互換性は Fedify に頼れない分、**フィクスチャ駆動の回帰テストとライブ相互運用テスト**（ADR-0006, ADR-0015）で担保する。
- Cloudflare のサービスのうち、R2（S3 互換 API）や CDN は他ターゲットと組み合わせて引き続き利用できる。
  Cloudflare Containers 上での実行は v0 の対象外とし、需要があれば別 ADR で検討する。
- 言語・ツールチェーン:
  - サーバー: Go（バージョンは `.tool-versions` で固定）。lint は `golangci-lint`。
  - Web クライアント・Tauri の UI: TypeScript。ツールチェーンは Bun（ADR-0028）。
  - 共有物（i18n カタログ・API スキーマ）は言語非依存のファイル形式（ICU MessageFormat の JSON、OpenAPI 等）で持ち、両言語から読む / 生成する。

## 経緯

RDB・キュー・ストレージ・実行環境を利用者が差し替えられること（G3, NFR-PORT-1〜4）が最重要の構造要件で、同時に小規模運営では「1 プロセス・1 DB」で完結する単純さ（NFR-COST-2）が要る。エージェントが局所的な変更で済む明確な境界も求めて、Ports & Adapters のモジュラーモノリスにした。

言語の候補は Go と Bun（TypeScript）で、ホスト先は Cloudflare / Google Cloud / AWS / Self-hosted の 4 系統を視野に入れていた（G2）。低コスト（G1）、Self-hosted のフットプリントと配布の容易さ（NFR-COST-3, NFR-DIST-2）、連合の正しさ（G4）、可搬性（G2, NFR-PORT）、クライアントとの共有（G5, G6）、コントリビュータとエージェントの生産性で比べた。TypeScript の主な利点は Cloudflare Workers をネイティブに支えられることだった。Workers を対象外とすると、残るターゲットではいずれもコンテナ / 単一バイナリが素直に動き、Go の軽さ・配布の容易さ・実行効率が上回る。

## 採らなかった案

- マイクロサービス: 「1 プロセス・1 DB」の単純さ（NFR-COST-2）を失う。
- 境界のないレイヤードなモノリス: 外部資源の差し替えとテストのしやすさ、エージェントが局所で済む境界が得られない。
- TypeScript（Web 標準 API 準拠コア + Bun）+ Fedify: workerd・Bun・Node で同一コードが動き、連合の互換性を Fedify に外部化できる。Self-hosted のメモリが Go より大きく、Workers の CPU 時間・バンドルサイズの制約を意識した実装が要る。
- ハイブリッド（Go コア + Cloudflare 用 TS シム）: Workers も支えられるが、2 言語・2 ランタイムで保守コストが最大になる。

## Consequences（影響）

- 新しいクラウドへの対応 = アダプタ + entrypoint の追加で済み、コアは変更不要。
- 抽象化の分だけボイラープレートは増える。
- Self-hosted / Cloud Run / Lambda（`provided.al2023`）/ k8s で軽量に動き、メモリは数十 MB、配布は単一の静的バイナリになる。サーバーは単一バイナリの中で `serve` / `worker` / `cli` を切り替える構成にでき、Self-hosted の最小構成は「バイナリ + SQLite ファイル」になる。
- 連合ライブラリは `go-ap` 程度で、採用実績のある汎用ライブラリは少ない。相互運用の細部は自前で詰める（GoToSocial は参考実装として有用）。連合の署名ライブラリと自前実装の範囲は ADR-0006、SQL ツールとドライバ（いずれも cgo 不要）は ADR-0003 に定める。
- Web / Tauri の UI は TypeScript になるため 2 言語体制で、i18n カタログと API 型の共有は生成ツールで行う。
- `CGO_ENABLED=0` の静的ビルドを維持する。

## Open Questions / 要検証

- Self-hosted のアイドル時メモリの実測（M1）
