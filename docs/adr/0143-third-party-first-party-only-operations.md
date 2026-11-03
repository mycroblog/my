# ADR-0143: サードパーティに開かない操作を増やす

- Status: Proposed
- Date: 2026-11-03
- Deciders: @rosylilly
- Issue: #2079

## Context（背景・課題）

[ADR-0090](0090-third-party-oauth-apps.md) §4 は、サードパーティのアプリに、現行のスコープの粒度をそのまま使わせます。例外は `admin*`・`write:supporters` と、アカウントの保安の操作（パスワード・二要素認証・メールアドレス・アカウントの削除・Move・認可したアプリの管理）です。

`v0.1.0` の前のクライアント API の監査（#2027）で、この一覧に無い操作が、細かいスコープと同意の画面の説明を飛び越えていると分かりました。

| 操作 | 求めるスコープ | 起きること |
| --- | --- | --- |
| データのエクスポート（`listDataExports`・`requestDataExport`・`downloadDataExport`） | `read:accounts` / `write:accounts` | 全公開範囲の投稿と各種の一覧を持ち出せる。`read:direct` を含む細かい読み取りのスコープが無意味になる |
| CSV のエクスポート（`exportCsv`） | `read:accounts` | `read:follows`・`read:blocks`・`read:mutes`・`read:bookmarks` を飛び越える |
| インポート（`startDataImport`・`previewDataImport`・`listDataImports`） | `read:accounts` / `write:accounts` | 1 万件までのフォロー・ブロックなどを置き換えられる。件数から非公開の一覧の大きさも分かる |
| 投稿の自動削除（`updateAutoDeletePolicy`・`previewAutoDeletePolicy`） | `read:accounts` / `write:accounts` | `write:statuses` なしで過去の投稿を消せる。連合先にも `Delete` が届き、戻せない。プレビューは `private` を含む投稿を返す |
| 別名の書き込み（`addAccountAlias`・`removeAccountAlias`） | `write:accounts` | 別のアカウントがフォロワーを移してくることへの同意を、アプリが与えられる |
| 招待（`listOwnInvites`・`createOwnInvite`・`revokeOwnInvite`） | `read:accounts` / `write:accounts` | 本人の名前で、審査を飛ばせる登録リンクを作り、読める |
| 異議申し立てと処分の一覧（`appealModerationAction`・`listOwnModerationActions`） | `read:accounts` / `write:accounts` | 1 回しかできない申し立てを使い切れる。処分の記録を読める |
| モデレーターの通知（`admin_report`・`admin_appeal`・`admin_sign_up`・`admin_error_group` など） | `read:notifications` | 権限を持つ人のトークンなら、管理の情報と関係するアカウントが届く。§4 の「管理は一次のクライアントだけ」に反する |

コードの読み取りによる確認で、実際のトークンでは試していません（#2027 の監査の記録）。

修正の issue は #2044 です。Accepted の ADR の本文は書き換えられないので、一覧を広げる決定をこの ADR に置きます。

## Decision Drivers（判断基準）

1. **同意の画面の説明が本当であること**（ADR-0090 §4）: 利用者が読んだ説明より多くのことをアプリができないこと。
2. **取り返しのつかない操作から利用者を守ること**: 連合先に届く削除、1 回限りの申し立て、一覧の置き換え。
3. **スコープの体系を増やさないこと**: 説明しきれないスコープを増やしても、1 を満たせない。
4. **`v0.1.0` の前に閉じること**: 後から開くのは追加で済むが、開いたものを閉じるのは ADR-0090 §8 の非推奨の手順がいる。
5. **正当なサードパーティの用途を残すこと**: バックアップや移行のアプリ。

## Considered Options（選択肢）

監査の修正案から順に、次の 4 つを比べました。

1. **A: 一次のクライアントだけにする** — 上の操作に `x-first-party-only` を付けます。通知は種類で絞ります。
2. **B: 専用のスコープを足し、同意の画面で警告する** — `account:export` などです。
3. **C: 既存の細かいスコープを追加で求める** — CSV は種類ごとに `read:follows` などを、自動削除は `write:statuses` と `read:statuses` を求めます。
4. **D: 変えず、スコープの説明を正直に書く** — `read:accounts` はこれらすべてを含むと書きます。

## Decision（決定）

> 推奨案です。Accepted にするのは人間の Decider だけです。

**A を推奨します。** 決定者は監査の推奨（Q1・Q2・Q5・Q6）を採っており、この ADR はその結論と理由を記録します。

ADR-0090 §4 の「スコープにかかわらず一次のクライアントだけにする」操作に、次を加えます。

- データのエクスポート: `listDataExports`・`requestDataExport`・`downloadDataExport`・`exportCsv`
- インポート: `startDataImport`・`previewDataImport`・`listDataImports`
- 投稿の自動削除の設定の変更とプレビュー: `updateAutoDeletePolicy`・`previewAutoDeletePolicy`（設定の読み取り `getAutoDeletePolicy` は `read:accounts` のまま）
- 別名の追加と削除: `addAccountAlias`・`removeAccountAlias`（`alsoKnownAs` として公開される情報なので、読み取り `listAccountAliases` は `read:accounts` のまま）
- 招待の一覧・作成・取り消し: `listOwnInvites`・`createOwnInvite`・`revokeOwnInvite`
- 異議申し立てと、本人の処分の一覧: `appealModerationAction`・`listOwnModerationActions`

モデレーターの通知は、操作を閉じるのではなく、サードパーティのトークンから種類で隠します。

- `notification.ModeratorPermission` に属する種類を、利用者の権限にかかわらず、`FirstParty` でないトークンの通知の一覧・ストリーミングで返しません。未読の位置（`notifications_marker`）の件数にも同じ規則を当てます。

理由は次のとおりです。

- B は判断基準 3 に反します。「すべての投稿と一覧を持ち出す」許可は、`read:accounts` の中の 1 つに分けても、同意の画面の 1 行で結果を説明できません。監査の修正案の最も強い対案ですが、警告を読んで許可した利用者にも、DM を含む持ち出しの結果は想像しにくいものです。
- C は CSV には当てはまりますが、データのアーカイブ（`moderation.json` を含む）と、申し立て・別名・招待には対応する細かいスコープがありません。自動削除に `write:statuses` を求めても、「投稿を書く」という同意の説明と、過去の投稿をまとめて消す結果が釣り合いません。
- D は判断基準 1 を説明の側で満たすだけで、判断基準 2 の害が残ります。
- 判断基準 5 の用途は失いますが、アプリは投稿・フォロー一覧などを細かいスコープで読めます。閉じる側の決定は、後から開くのが追加で済むので、戻せます（判断基準 4）。

### この決定の範囲の外

監査の同じ issue（#2044）の残りは、ADR-0090 の変更がいらないので、この ADR では決めません。`read:follows` がブロックとミュートの状態を返さないこと、使われていない `read:favourites` を除くこと、`read:accounts` と `write:accounts` の説明を残ったものに合わせて書き直すことです。

### この決定を見直す条件

- バックアップや移行のアプリの要望が実際に届き、同意の画面で結果を説明できる方法が見つかったら、エクスポートだけを B で開きます。
- 通知の種類ごとのスコープを作ることになったら、モデレーターの通知をそちらへ移せないかを見直します。

## Consequences（影響）

- 良い影響
  - `read:direct` と細かい読み取りのスコープが、エクスポートで飛び越えられなくなります。
  - アプリが、利用者の投稿を消す・申し立てを使い切る・登録リンクを作ることができなくなります。
  - `read:accounts` と `write:accounts` の説明が、短く正直に書けます。
- 悪い影響・トレードオフ
  - サードパーティのバックアップ・移行のアプリは、データのアーカイブを使えません。
  - 招待の管理や申し立てをサードパーティのクライアントからできません。Web クライアントを開く必要があります。
  - 通知だけは「操作を閉じる」でなく「種類で隠す」ので、規則が 2 種類になります。
- 後続で必要になる作業（Issue 化する）
  - #2044: `x-first-party-only` の追加、通知の絞り込み、スコープと同意の文言、テスト。`api/spec/openapi.yaml` の一次のクライアントの説明も直します。
  - ADR-0090 の Status に、§4 の一覧をこの ADR が加えることを書きます（Accepted になるとき）。

## Open Questions / 要検証

- 要検証: 監査はコードの読み取りです。#2044 の実装で、実際のサードパーティのトークンで 403 と通知の絞り込みを確かめます。
