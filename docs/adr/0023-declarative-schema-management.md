# ADR-0023: DB スキーマは宣言的に管理し、Atlas（Community Edition）で適用する

- Status: Accepted（v0 の破壊的変更の扱いを ADR-0087（実運用開始後のスキーマ変更とデータの引き継ぎ）が、スキーマの適用を Atlas の CLI で行い単一バイナリに組み込まないことを ADR-0126（スキーマの適用を my に組み込み、Atlas の Go ライブラリで my schema apply する）が置き換える）
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: —

## Context

DB スキーマは方言ごとに `schema/<dialect>.sql` を持つ（ADR-0003）。v0 はマイグレーション履歴を持たず、破壊的変更を許容する（ADR-0012）。
このスキーマファイルを実際の DB にどう反映するかを決める必要がある。

- 毎回 DB を作り直す（`db reset`）だけでは、開発中の DB やテスト運用のインスタンスのデータを、変更のたびに失う。
- 手書きのマイグレーション SQL は、スキーマファイルとの二重管理になり、方言ごとに書き分ける手間も増える。
- 「あるべきスキーマ」をファイルで宣言し、現在の DB との差分をツールに計算させれば、スキーマファイルだけを正として扱える。

## Decision Drivers

1. スキーマの正が 1 つ（方言ごとの目標スキーマファイル）であること
2. SQLite と PostgreSQL の両方に対応していること
3. OSS（AGPL-3.0）のコントリビュータが、無償・ログインなしで同じ手順を使えること
4. 目標スキーマを sqlc（ADR-0003）とも共有できる形式（SQL）で書けること
5. v1 以降に、目標スキーマからバージョン付きのマイグレーションを生成する道があること

## Considered Options

1. **Atlas Community Edition**（Apache-2.0）: 目標スキーマを SQL または HCL で書き、`atlas schema apply` で差分を計算して適用する。差分は開発用 DB（dev database）に目標スキーマを実際に流して正規化してから計算する。`atlas migrate diff` で、目標スキーマからバージョン付きのマイグレーションも生成できる。
2. Atlas 標準配布版: Community Edition の機能に加え、`schema plan` / `schema lint` や、ビュー・トリガー・拡張機能などを扱える。ただし一部の機能は `atlas login` と、試用期間後のライセンスが必要。
3. sqldef（MIT。同梱の SQL パーサは Apache-2.0）: 目標スキーマを SQL で書き、自前のパーサで差分を計算する。開発用 DB は要らないが、パーサが対応していない構文は扱えない。
4. 宣言的にせず、手書きのマイグレーション（goose など）。

## Decision

**1. Atlas Community Edition を採用し、DB スキーマを宣言的に管理する。**

- **スキーマの正は、方言ごとの目標スキーマファイル `schema/sqlite.sql` / `schema/postgres.sql`（SQL）** とする。
  - 同じファイルを sqlc のスキーマ入力にも使う。
  - 論理データモデル（`docs/architecture/data-model.md`）との乖離は CI で検査する（ADR-0003）。
- **スキーマの変更は目標スキーマファイルの編集だけで行う。** マイグレーション SQL は手で書かない。
- 適用は `atlas schema apply`（目標スキーマ → 対象 DB）で行う。
  - 開発用 DB は、SQLite ならインメモリ（`sqlite://dev?mode=memory`）、PostgreSQL ならコンテナを使う。
  - 差分の確認は `atlas schema diff` / `atlas schema apply --dry-run` で行う。
- Makefile に `db-apply`・`db-diff`・`db-reset` などのターゲットを用意し、Atlas の CLI を直接叩かなくても済むようにする（M1）。
- **使うのは Community Edition だけ** とする。標準配布版の機能や `atlas login` が必要な機能に依存しない。
  - CI とコンテナイメージには Community Edition のバイナリを入れる（`arigaio/atlas:<version>-community`、または `atlasgo.sh` の `--community`）。
  - バージョンは固定する。
- **Community Edition で扱えないもの**（拡張機能、シーケンス、パーティショニング、ビュー、トリガーなど）は、目標スキーマに書かない。
  - 必要な拡張機能（PostgreSQL の `pg_trgm` など、ADR-0009）は、Atlas の前に実行する冪等な準備 SQL（`schema/postgres.bootstrap.sql`、`CREATE EXTENSION IF NOT EXISTS …`）で作る。
  - ID は UUIDv7 をアプリで生成するので（ADR-0003）、シーケンスは使わない。
- **v0 の破壊的変更**（ADR-0012）:
  - カラムの削除など、データを失う変更も `schema apply` で反映してよい。
  - 差分の適用に失敗する場合や、整合性を保てない場合は、`db-reset`（DB を作り直してから適用）を使う。
- **v1 以降**: 目標スキーマは引き続き正とし、`atlas migrate diff` で目標スキーマからバージョン付きのマイグレーションを生成して、本番に適用する。生成物の運用（レビュー・適用手順）は、v1 に向けた ADR で決める。

## Consequences

- スキーマの変更は、目標スキーマファイルの差分としてレビューできる。手書きのマイグレーションとの二重管理がない。
- 開発中やテスト運用のインスタンスでも、変更のたびに DB を作り直さずに済む。
- Community Edition には `schema lint` / `schema plan` がないので、破壊的変更の検出は `schema apply --dry-run` の出力を CI で確認する形で行う。
- Aurora DSQL のドライバは Community Edition にない。DSQL を使う場合に Atlas の PostgreSQL ドライバで適用できるかは、未検証である（ADR-0003 の Open Questions の DSQL 検証に含める）。
- サーバーの単一バイナリには Atlas を組み込まない。スキーマの適用はデプロイ手順の一部として、Atlas の CLI で行う。
  - Self-hosted の最小構成でも、Atlas の CLI（またはそれを含むコンテナイメージ）が必要になる。
  - デプロイテンプレート（ADR-0010）に、適用ステップとして含める（docker compose の一回限りのサービス、Kubernetes の Job / initContainer、Cloud Run のジョブなど）。

## 参考

- [Atlas](https://atlasgo.io/) / [Community Edition](https://atlasgo.io/community-edition)
- [sqldef](https://github.com/sqldef/sqldef)
