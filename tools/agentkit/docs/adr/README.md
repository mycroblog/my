# agentkit の ADR

agentkit（`tools/agentkit`）の決定記録。本体の `docs/adr/` とは別に管理し、本体の ADR の索引には載せない（[ADR-0001](0001-agentkit.md)）。
書き方は本体の [テンプレート](../../../../docs/adr/template.md) と [`/adr` スキル](../../../../.claude/skills/adr/SKILL.md) に従う。Status を Accepted / Rejected にするのは決定者だけ。

| ADR | タイトル | Status |
| --- | --- | --- |
| [0001](0001-agentkit.md) | agentkit をリポジトリ内の独立した Go モジュールとして置く | Accepted |
| [0002](0002-knowledge-graph.md) | 構造の索引と全文検索で前提を引き、結果は CLI で小さく返す | Accepted |
| [0003](0003-context-map.md) | パスと規約の対応表 context-map を、コードを増やすエージェントが保守する | Accepted |
| [0004](0004-claude-code-hooks.md) | Claude Code のフックで、高くつく読み方を止め、長い出力を切り詰める | Accepted |
| [0005](0005-github-cache-and-workflow.md) | GitHub のデータをローカルにキャッシュし、ワークフローの手順をラップする | Accepted |
