# 画面の要件（L1）

画面（ルート）ごとに 1 ファイル `<slug>.md` を置く（[ADR-0082](../../../adr/0082-conflict-free-file-layout.md)）。
[ADR-0110 §4](../../../adr/0110-web-experience-principles.md) の L1 で、[体験の原則](../21-experience-principles.md)を画面に当てはめた短いテキストにする。
書式は Shape Up の Breadboarding（場所・操作・つながり）と OOUX の ORCA（Objects・Relationships・CTAs・Attributes）を下敷きにする。

- UI を変える issue は、その画面のこのファイルがなければ `agent:ready` にしない。画面を変える PR は、挙動が変わるならこのファイルも同じ PR で直す。
- 書いたら、その画面の機能の entry の `docs`（`tools/agentkit/context-map.yaml`）にファイルを足す。`agentkit ctx` がその画面の issue に要件を入れる。
- 現在の状態だけを書く（[ADR-0021](../../../adr/0021-documents-describe-present-state.md)）。

## 雛形

```markdown
## <画面の名前>（`/path`）

### 1. 使う人と頻度
誰が、何のために、どのくらいの頻度で使うか。

### 2. 作業
| 作業 | 最短の手順の数 |
| --- | --- |
| （L4 の課題になる。1 行 1 作業） | N |

### 3. 情報の優先順位とまとまり
区画の見出し（`h2`）を、優先順位の順に並べる。L3 が実装の `h2` の順と比べる。
1. <区画>: 入るもの

### 4. 主な操作と保存の仕方
主な操作は 1 つ。保存は、すぐ保存か、まとめて保存かのどちらか 1 つ。

### 5. 操作ごとの結果の出し方
| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |

### 6. 入口と出口
どこから来て、終えたらどこへ行くか。
```

## 画面の一覧

- [バッジとサポーターの一覧（`/settings/badges`）](badges.md)
- [サーバーの設定（`/admin/settings`）](admin-settings.md)
- [ホームタイムライン（`/home`）](home.md)
- [ランディングページ（`/`、サインインしていない人）](landing.md)
- [ローカルタイムライン（`/local`）](local.md)
- [連合タイムライン（`/public`）](public.md)
- [設定のメニュー（`/settings` と各画面の横）](settings-menu.md)
- [設定のトップ（`/settings`）](settings.md)
- [表示・投稿・タイムライン（`/settings/preferences`）](settings-preferences.md)
- [通知（`/settings/notifications`）](settings-notifications.md)
- [プロフィール（`/settings/profile`）](settings-profile.md)
- [古い投稿の自動削除（`/settings/auto-delete`）](settings-auto-delete.md)
