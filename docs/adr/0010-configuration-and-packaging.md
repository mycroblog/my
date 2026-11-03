# ADR-0010: 設定・パッケージング・デプロイテンプレート

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #11

## Context

利用者が自由にカスタマイズでき（G3）、3 系統のターゲットに配れる（G2, NFR-DIST-2）必要がある。

## Decision

### 設定

- 単一の型付き設定スキーマを `core` に定義し、起動時に検証（NFR-PORT-5）。
- ソース: 環境変数（`MY_` プレフィックス、名称はプロダクト名確定時に変更）→ 任意の設定ファイル（TOML / JSON）→ 既定値。
- アダプタ選択は明示的なキー（例: `MY_DATABASE=postgres`, `MY_DATABASE_URL=...`, `MY_QUEUE=sqs`, `MY_BLOB=s3`）。URL スキームからの推測はしない。
- 秘密情報は環境変数 / シークレットストア（Secret Manager, Secrets Manager / SSM, Kubernetes Secret）から。

### パッケージング

- モノレポ。Go は単一モジュール（`cmd/`, `internal/`, `pkg/testkit`）とする。クライアントアプリは `app/` の下に 1 アプリ 1 ディレクトリで置く（`app/web`、`app/mobile`）。
  `app/*` を Bun ワークスペースのメンバーにし、クライアントのツールチェーン（依存・lint・テスト）をサーバーの Go と分けて扱う（レイアウトは [overview](../architecture/overview.md#6-リポジトリ構成)）。
- アダプタはパッケージ単位に分け、ビルドタグで除外できるようにする（NFR-PORT-4）。
- エントリポイント: `cmd/my`（単一バイナリ: `serve` / `worker` / `cli`。Self-hosted・Cloud Run・コンテナ）、`cmd/my-lambda`（AWS Lambda。Lambda Web Adapter で `cmd/my` を流用できるなら不要）。
- コンテナイメージ: multi-arch（amd64 / arm64）、非 root、distroless 系。GHCR に公開。
- 利用者による拡張: 独自アダプタは「ポートの型を実装し、コントラクトテストを通し、独自エントリポイントで組み立てる」方式。
  Go の `plugin` パッケージによる動的ロードは使わない（移植性が低いため）。v0 ではフォークまたはツリー内への追加で拡張する。

### デプロイテンプレート（`deploy/`）

| ターゲット | 形式 |
| --- | --- |
| Docker | `compose.yaml`（SQLite 最小構成 / PostgreSQL + MinIO 構成） |
| microk8s / k8s | Kustomize（base + overlays）。Helm chart は需要次第 |
| Google Cloud | OpenTofu モジュール（Cloud Run, Cloud Tasks, GCS, Scheduler） |
| AWS | OpenTofu モジュール（Lambda, SQS, S3, CloudFront, EventBridge Scheduler） |

IaC は OSS ライセンスの OpenTofu を採用（Terraform 互換）。

## Consequences

- 設定スキーマが「どのアダプタが何を要求するか」のドキュメントを兼ねる（スキーマからドキュメント生成）。
- テンプレート 5 種の保守負荷。M6 で各ターゲットの E2E スモークテストを nightly 化する。
