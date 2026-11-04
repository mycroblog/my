# 論理データモデル（v0）

> 方言に依存しない **論理スキーマ** です。方言別の `schema/<dialect>/<NN>-<module>.sql`（[§4](04-tables/README.md) のモジュールごとのファイル）はこの文書に従って書き、乖離は CI で検査します（[ADR-0003](../../adr/0003-database-abstraction.md)）。
> モデリングの規約は [ADR-0020](../../adr/0020-data-modeling-principles.md)（リソース・イベント・投影の分離と、状態の直交分解）です。この文書はその適用結果です。
> 前提にしている ADR: [ADR-0002](../../adr/0002-modular-monolith-hexagonal.md)、[ADR-0003](../../adr/0003-database-abstraction.md)、[ADR-0004](../../adr/0004-job-queue-abstraction.md)、[ADR-0016](../../adr/0016-client-api-style.md)、[ADR-0013](../../adr/0013-timeline-strategy.md)、[ADR-0017](../../adr/0017-visibility-local.md)、[ADR-0018](../../adr/0018-federated-uris.md)。
> マイグレーション履歴は持ちません。`v0.1.0` より前は作り直し、以降はデータを失わない変更だけを許します（[ADR-0023](../../adr/0023-declarative-schema-management.md)）。

## 節の一覧

- [1. 共通規約](01-conventions.md)
- [2. 整合性モデル](02-consistency.md)
- [3. 概念モデル](03-concepts.md)
- [4. モジュール別テーブル定義](04-tables/README.md)
- [5. 状態とポリシー](05-states-and-policies.md)
- [6. タイムラインのクエリ](06-timeline-queries.md)
- [7. 公開範囲と配送](07-visibility-and-delivery.md)
- [8. 削除と消去](08-deletion-and-purge.md)
- [9. 投影の再構築と検証](09-projection-rebuild.md)
- [10. 機能要件とテーブルの対応](10-requirements-mapping.md)
- [11. 未決事項](11-open-questions.md)
