# ADR-0051: カスタム絵文字とリアクション

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #618, #619, #1469

## Decision（決定）

カスタム絵文字は Mastodon 相当の範囲にする。`content` の既存のテーブルにローカルの行と設定を足し、登録・画像の差し替え・カテゴリ・別名・ピッカーに出すか・無効化・削除・リモートの複製を、管理 API・CLI・管理画面で行う。ローカルの絵文字の `id` は `https://{host}/ap/emojis/{uuid}` にする。

リアクションはお気に入りと別の資源にし、ローカルの人の `Like` の枠を 1 投稿に 1 つにする（リモートの人の複数のリアクションは残す）。送信は相手のソフトウェアを見ず、常に `content` と `_misskey_reaction` 付きの `Like` にする。

### 1. データモデル（`content` が持つ。ADR-0020）

| テーブル | 分類 | カラム | 制約・規則 |
| --- | --- | --- | --- |
| `custom_emojis` | リソース | `id`, `shortcode`, `domain`（ローカルは `''`）, `uri`（NULL 可）, `created_by_user_id`（NULL 可）, `created_at` | 一意 `(shortcode, domain)`, `(uri)`。ローカルの `uri` は NULL で、`id` から `https://{host}/ap/emojis/{id}` を導く（ADR-0018 の `Create` の `id` と同じく保存しない）。`created_by_user_id` は作った利用者で、CLI とリモートの絵文字は NULL、承認で作った絵文字は申請者、管理画面で作った絵文字と複製はその管理者。変えない（ADR-0070 §7） |
| `custom_emoji_images` | イベント（版） | `id`, `emoji_id`, `media_id`, `revised_at` | 最新の行が現在の画像。ローカルはアップロードした `media_files`、リモートは `remote_media`。画像の差し替えは行を足す |
| `custom_emoji_settings` | 現在値 | `emoji_id`（PK）, `enabled`（`bool`）, `visible_in_picker`（`bool`）, `category`（NULL 可、64 文字まで） | 行が無ければ `enabled = true`・`visible_in_picker = true`・カテゴリなし。リモートの絵文字は `enabled` だけを使う |
| `custom_emoji_aliases` | 現在値 | `emoji_id`, `alias`（shortcode と同じ文字種） | PK `(emoji_id, alias)`。ローカルだけ。ピッカーの検索に使う |
| `account_profile_revision_emojis` | イベント（版の子） | `revision_id`, `emoji_id` | PK `(revision_id, emoji_id)`。`account` が持ち、`emoji_id` は `content` の公開 API で得る |

- 状態（無効・ピッカーに出さない）は `custom_emoji_settings` の現在値で持ち、nullable な時刻で表さない。変更の履歴は監査ログ（§5）が持つ。
- **shortcode**: ローカルは `[A-Za-z0-9_]{2,30}`（Mastodon の文字種と GoToSocial の長さの上限に合わせる。要検証: GoToSocial の上限）。大文字と小文字を区別し、変えられない（投稿の本文が shortcode で参照するため）。
  リモートは受け取った値を `:` を外して保存し、長さは 128 文字までに切る（超えたものは捨てる）。
- **削除**: ローカルの絵文字の削除は、`custom_emojis`・`custom_emoji_images`・設定・別名の行と画像の Blob を同じトランザクションで物理削除する（Blob は outbox のジョブで消す）。
  過去の版の `status_revision_emojis` / `account_profile_revision_emojis` の行は残し、読み取りでは絵文字の無い参照として飛ばす（本文は `:shortcode:` のまま表示される）。
  同じ shortcode で作り直した絵文字は新しい `id` なので、過去の投稿には付かない。
- **リモートの絵文字の識別**: `domain` は投稿・プロフィール・リアクションの作者（送り手の Actor）の正規化したホストで、`(shortcode, domain)` で引く（Mastodon・Misskey と同じ）。リアクションの `Emoji` の `id` が別のホストのときの扱いは §6 に従う。
  `uri` は `Emoji` の `id` のホストが `domain` と同じときだけ記録し、違うとき・無いときは NULL にする。他のサーバーの絵文字の `id` を名乗って行を奪わせないため。
  画像の URL が記録と違えば `custom_emoji_images` に行を足す。
- **リモートの絵文字の消去**: どの版（投稿・プロフィール）とリアクションからも参照されなくなったリモートの絵文字は、1 日ごとのジョブ `content.purge_remote_emojis` が消す（FR-803、ADR-0006 の保持期間で投稿が消えた後に残らないように）。管理者が無効にしたものは、参照が無くても残す。消すと、同じ絵文字を次に受けたときに有効な行として作り直し、無効にした判断が失われるからである。

### 2. 画像の制約（ADR-0005、ADR-0019 §8）

- 形式: PNG（APNG を含む）・GIF・WebP。中身で判定し、JPEG・AVIF・SVG は絵文字として受け付けない（Mastodon が絵文字として扱う形式に合わせる）。
- ローカルのアップロード: 運用者の設定 `custom_emoji_max_bytes`（既定 256 KiB、Mastodon の上限）以下、長辺 1,024 px 以下。アニメーションのフレーム数の上限は ADR-0019 §8 の値を使う。
  GoToSocial は既定で 100 KiB を超えるリモートの絵文字を取得しないので、管理画面は 100 KiB を超える画像に注意を表示する。
- 派生は ADR-0005 の用途 `emoji` で作る: S / M / L は 64×64 / 128×128 / 256×256 の箱に縦横比を保って収め、アニメーションは静止版（最初のフレーム）と動く版の両方を作る。原本は保存しない。
  形式も ADR-0005 §5, §6 に従う（L は PNG、動く版の L は GIF）。APNG は ADR-0005 §6 のとおり最初のフレームの静止画として扱い、アニメーション WebP は ADR-0005 の spike が通ったときだけ受け付ける。
- リモートの絵文字の画像はメディアプロキシ（federation.md §9）を通し、静止版はプロキシの `small`（ADR-0005 §10。絵文字の S の 64×64）で返す。プロキシは、`custom_emoji_images` が指すメディアには設定 `remote_custom_emoji_max_bytes`（既定 1 MiB）の上限を当て、超えるものを 404 にする。
  形式の許可リストは上の 3 つに絞る。

### 3. 連合（送信・受信）

**`Emoji` タグの形**（投稿・Actor・リアクションで共通）:

```json
{
  "id": "https://social.example.org/ap/emojis/0192f3d0-1a2b-7c3d-8e4f-5a6b7c8d9e0f",
  "type": "Emoji",
  "name": ":blobcat:",
  "updated": "2026-10-20T03:04:05Z",
  "icon": {
    "type": "Image",
    "mediaType": "image/png",
    "url": "https://media.social.example.org/media/0192f3d1-.../0192f3d2-.../large.png"
  }
}
```

- `id`: ローカルの絵文字は `https://{host}/ap/emojis/{uuid}`（`{uuid}` は `custom_emojis.id`）。リモートの絵文字を参照するときは記録した `uri` で、NULL なら `id` を省く。
- `name`: `:` で囲んだ shortcode。
- `updated`: 現在の画像の `custom_emoji_images.revised_at`（RFC 3339、UTC、秒まで）。画像を差し替えると変わり、相手が取り直す。
- `icon`: `type` は `Image`、`mediaType` は `url` の形式（ローカルは派生の `content_type`）、`url` はローカルなら L の公開 URL（アニメーションなら動く版の L の GIF。ADR-0005 §8 の Actor の `icon` と同じ考え方）、リモートなら記録した元の URL（メディアプロキシの URL は送らない）。
- `@context` には `{"toot": "http://joinmastodon.org/ns#", "Emoji": "toot:Emoji"}` を入れる。

カスタム絵文字の URI は ADR-0018 の URI 一覧にあり、受信した自サーバーの URI のパース（`^/ap/(actors|objects|activities|emojis)/{uuid}`）も `emojis` を含む。ADR-0018 の他の規則は変えない。

- `GET /ap/emojis/{uuid}` は上の `Emoji` を（`@context` を付けて）返す。存在しない・削除済み・無効の絵文字は同じ 404 にする。HTML を求めるリクエストも 404。
- **どこに付けるか**: Note（`Question` を含む）の `tag` には、その版の本文・CW（`summary`）・投票の選択肢（`name`）で使った絵文字を、Actor の `tag` には表示名・自己紹介・補足情報の名前と値で使った絵文字を入れる。
- **ローカルの本文からの抽出**: 投稿・プロフィールの保存のとき、サーバーがプレーンテキストの原文から `:shortcode:` を探し、有効なローカルの絵文字だけを版に記録する（無効・未登録の shortcode は文字のまま）。
  前後の境界は Mastodon の `CustomEmoji::SCAN_RE`（前後が英数字と `:` でないこと）に合わせる。ローカルの投稿で使えるのはローカルの絵文字だけ。
- **受信**: 投稿の `tag` の `Emoji` は `status_revision_emojis` に記録する。Actor の `tag` の `Emoji` は、プロフィールの版を作るときに同じ規則（§1 の識別）で記録し、`account_profile_revision_emojis` に結ぶ。
  1 つの文書から記録する絵文字の数は、既存の `MaxTags` の上限に従う。

### 4. クライアント API と表示

- 絵文字の参照（API の `CustomEmoji`）: `shortcode`（`:` なし）, `url`, `static_url`。ローカルは `url` が M（アニメーションなら動く版の M）、`static_url` が静止版の M の公開 URL。
  リモートは `url` がメディアプロキシの `full`、`static_url` が `small`。リモートの絵文字には寸法と派生の記録がないので、ADR-0005 の `MediaVariants` ではなくこの 2 つの URL にする。
- `Status.emojis` はその版の本文・CW・投票の選択肢の絵文字、`Account.emojis` は現在のプロフィールの版の表示名・自己紹介・補足情報の絵文字。通知は埋め込む Status / Account がそのまま持つ。
- 返さないもの: 無効（`enabled = false`）の絵文字、削除された絵文字、画像の所有者のドメインか URL のホストに `media_fetch` の制限（`reject_media`）があるリモートの絵文字。返さない絵文字は文字の `:shortcode:` として表示される。
- ピッカーの一覧 `GET /custom_emojis`: 有効でピッカーに出すローカルの絵文字を、`category`・`aliases` 付きで返す。キャッシュは ADR-0028 の方針に従う。
- 管理 API `/admin/custom_emojis`: 一覧（ローカル・リモート、shortcode とドメインで絞り込み）、作成（アップロード済みのメディアと shortcode）、画像の差し替え、設定と別名の変更、削除、リモートの絵文字のローカルへの複製（新しい shortcode を指定できる）。申請の審査と表現の `created_by` は ADR-0070 §6 が定める。
  エンドポイントの細部は [client-api.md](../architecture/client-api.md) の手順で決める。
- **Web の描画**: サニタイズ済み HTML は DOM のテキストノードを、プレーンテキストのフィールド（表示名・CW・投票の選択肢・補足情報）は文字列を、同じ字句解析で `:shortcode:` に分け、`emojis` にある shortcode だけを
  `<img alt=":shortcode:" title=":shortcode:" draggable="false">` の DOM 要素として差し込む（`innerHTML` は使わない。ADR-0019 §2）。
  高さは周りの文字に合わせる（デザイントークン）。アニメーションは ADR-0005 §6 と同じく `<picture>` の `<source media="(prefers-reduced-motion: no-preference)">` に `url` を、`<img>` に `static_url` を置き、利用者の設定「アニメーションを自動再生しない」が有効なら `<source>` を出さない。
- **ピッカー**: 投稿の作成と編集、プロフィールの編集に置く。カテゴリごとの一覧と、shortcode・別名の検索を持ち、選ぶとカーソル位置に `:shortcode:` を入れる。
  最近使った絵文字はクライアントの端末ごとに持つ。リアクションで使える絵文字は §7 が決める。ピッカーの構成は ADR-0091 が決める。

### 5. 権限と監査ログ

- 権限 `manage_custom_emojis` を置く。組み込みの `admin` ロールは持ち、`moderator` ロールは持たない（Mastodon と同じ）。区分はサーバー管理（ADR-0058 §4）。管理 API・CLI・管理画面の絵文字の操作はこの権限で行う。
  ただし、ローカルの絵文字の作成は `create_custom_emojis` の申請と承認（ADR-0070 §2）でも行い、作成者は自分の絵文字の一部の操作（ADR-0070 §3）を行う。リモートの絵文字の無効化は `manage_custom_emojis` で行う。
- 監査ログ（ADR-0058）に、作成・画像の差し替え・設定と別名の変更・無効化と有効化・削除・リモートの複製を記録する。
  同じ操作で ADR-0060 の webhook のイベントを出す: 作成とリモートの複製は `custom_emoji.created`、画像の差し替え・設定と別名の変更・無効化と有効化は `custom_emoji.updated`、削除は `custom_emoji.deleted`。`data` は管理 API `/admin/custom_emojis` の表現。受信でリモートの絵文字を記録したときは出さない。
  ADR-0058 の `target_kind` に `custom_emoji` を加え、`target_label` は `:shortcode:`（リモートは `:shortcode:@domain`）にする。ADR-0058 の他の規則は変えない。
- CLI は `my admin emoji` の下に、作成（ファイルと shortcode）・一覧・画像の差し替え・カテゴリと別名とピッカーへの表示の変更・無効化と有効化（リモートの絵文字を含む）・削除・リモートの絵文字のローカルへの複製を持ち、管理 API と同じアプリ層の操作で行う。

### 6. リアクションの資源とお気に入りとの関係

- リアクションは `content` モジュールの現在値 `reactions` にする（ADR-0020）。行は 1 つのリアクションで、主体・投稿・絵文字・`uri`（`Like` / `EmojiReact` の `id`）・作成時刻を持つ。一意制約は `(account_id, status_id, 絵文字)` と `(uri)`。
  絵文字は「Unicode 絵文字の文字列」か「`custom_emojis` の行」のどちらかで、集計に使う正規化した鍵（Unicode はその文字列、カスタム絵文字は `custom_emojis.id`）を持つ。
- 取り消しは行の削除と `content.unreacted` イベント（お気に入りと同じく、取り消した後の付け直しは新しい行）。絵文字ごとの件数は `content` が持つ投影（`status_reaction_counts (status_id, 絵文字の鍵, count)`）で、行を実際に挿入・削除したときだけ増減する。表と列の詳細は [data-model.md](../architecture/data-model/README.md) に書く。
- `favourites` と `favourites_count` はリアクションと別に残す。お気に入りとリアクションは別の資源で、API でも別に数える。
- **ローカルの人**は 1 投稿にお気に入りかリアクション 1 つのどちらか一方だけを持つ（`Like` の枠）。リアクションを付けると、同じ投稿へのお気に入りや別の絵文字のリアクションを同じトランザクションで外し、外したものの `Undo` を送ってから新しい `Like` を送る（どちらもアウトボックス経由）。お気に入りを付けたときも同じ。
  Mastodon は `Undo Like` で `id` に関係なくお気に入りを消し、Misskey は 1 人 1 投稿にリアクションを 1 つしか持たないので、枠を 2 つ使うと相手のサーバーでの状態が食い違う。
- **リモートの人**は、相手のサーバーが送ったとおりに記録する。Akkoma の人のお気に入りと絵文字ごとの複数のリアクションは、`favourites` と `reactions` の別々の行になる。
- 受信の振り分けは次のとおり。

| 受信 | 記録 |
| --- | --- |
| `EmojiReact`（`content` が絵文字） | リアクション |
| `Like` で `_misskey_reaction` か `content` が絵文字（FEP-c0e0、Akkoma と同じ判定） | リアクション |
| それ以外の `Like` | お気に入り |
| `Undo` | 対象の `uri` で `reactions`、`favourites` の順に引いて消す。どちらにも無ければ、埋め込みの `object` を上の表で振り分け、リアクションならその主体のその投稿への同じ絵文字のリアクションを、お気に入りならその主体のお気に入りを消す |

- 受け入れる絵文字は次のとおり。外れた `EmojiReact` は捨て、外れた `Like` は Misskey と同じくお気に入りとして記録する（Mastodon の利用者から見た件数と合わせる）。
  - Unicode 絵文字: 1 書記素の絵文字（FEP-c0e0）。異体字セレクタの有無などの揺れは、Unicode の `emoji-test.txt` から生成した表で fully-qualified の形に正規化して集計を揃える（Pleroma も同じ方向で正規化する。[MR !3708](https://git.pleroma.social/pleroma/pleroma/-/merge_requests/3708)）。表はリポジトリ内で生成し、外部ライブラリを足さない。
  - カスタム絵文字: `content` が `:shortcode:` で、`tag` に `name` が一致する（コロンの有無を問わない。Akkoma はコロンなし、Misskey と Mastodon はコロン付き）`Emoji` が 1 つあるもの。Iceshrimp.NET 系の `name@host` の接尾辞は落とす（Akkoma と同じ）。
- リアクションのカスタム絵文字のドメインは、`Emoji` の `id` のホスト。`id` が無ければ、リアクションの主体のドメイン。絵文字の行はドメインごとに次のように扱う。
  - ドメインが主体のドメインなら、§1 の規則で `custom_emojis` に画像と一緒に取り込む。
  - ドメインがこのサーバーなら、ローカルの絵文字を `(shortcode, '')` で引く。
  - それ以外（第三のサーバー）なら、`(shortcode, そのドメイン)` で記録済みの行があるときだけ使う。リアクションからは行も画像も作らず、変えない。行が無ければリアクションを捨てる（`EmojiReact` も、絵文字付きの `Like` も同じ）。あるサーバーの絵文字の行と画像は、そのサーバーの文書だけが決める。

### 7. ローカルの人が使える絵文字

- Unicode 絵文字と、このサーバーで有効なローカルのカスタム絵文字（§1 の `custom_emoji_settings.enabled`）。
- リモートのカスタム絵文字ではリアクションできない。その投稿にすでに付いているリアクションに同じ絵文字で加わることもしない。Misskey は送り手のホスト以外の絵文字を ❤ に置き換えるので、送っても相手に同じ絵文字として届かないため。

### 8. リアクションの送信の形

- リアクションは、FEP-c0e0 の互換の形として、**作者の inbox へ `content` と `_misskey_reaction` を付けた `Like`** を送る。宛先・署名・配送はお気に入りの `Like` と同じ（[federation.md §4](../architecture/federation.md)）。
- `content` と `_misskey_reaction` は同じ文字列で、Unicode ならその絵文字、カスタム絵文字なら `:shortcode:`。カスタム絵文字は `tag` に `Emoji` を 1 つ付ける。形は §3 に従う（`toot:Emoji`。`id`・`type: "Emoji"`・コロンで囲んだ `name`・`updated`・`icon` の `Image`（`mediaType` と `url`））。`@context` には `toot` と `misskey`（`https://misskey-hub.net/ns#`）の `_misskey_reaction` を足す。
- 取り消しは `Undo`（`object` に元の `Like` を埋め込む）。
- この 1 つの形で、Misskey（`_misskey_reaction`）と Pleroma / Akkoma（`content` 付きの `Like` を `EmojiReact` に書き換える）ではリアクション、Mastodon と GoToSocial ではお気に入りとして届く。
- `EmojiReact` は送らない。受信は ADR-0006 の互換拡張に、LitePub の `EmojiReact` と Misskey の `_misskey_reaction` を加える（ADR-0006 の他の部分は変えない）。投稿の `emojiReactions` コレクション（Fedibird の拡張）は公開しない。
- `local` の投稿（ADR-0017）とローカルの人の投稿へのリアクションは連合しない（お気に入りと同じく、リモートの投稿へのものだけを送る）。

### 9. 公開範囲・ブロック・モデレーション

- リアクションを付けられる投稿は、お気に入りと同じ単一のポリシー関数（`canView`。リモートの人は `ContextObjectFetch`）で閲覧できる、削除されていない投稿。ブロックしている・されている相手の投稿には付けられず、受信したものも捨てる。
- 交流の禁止（FR-708、`account_restrictions` の `interact`）はリアクションも禁止する。
- 投稿の削除・アカウントの削除・凍結での消し方は `favourites` と同じ扱いにする（[data-model.md](../architecture/data-model/README.md) のアカウント削除の表）。

### 10. リアクションの通知・クライアント API・Web

- 通知の種類に `reaction` を足す（FR-205）。お気に入りと同じく、ミュート・ブロック・通知の除外の規則に従う。同じ人が別の絵文字で付け直したときは、新しい通知を作る。
- クライアント API（ADR-0016、パスと名前は [client-api.md](../architecture/client-api.md) の規約に従う）:
  - `Status` に、絵文字ごとの件数と自分が付けたかどうかの集計（件数の多い順に最大 20 種類）と、カスタム絵文字の表示に要る情報（ショートコード・画像の URL・ドメイン）を持たせる。
  - 自分のリアクションを付ける（置き換える）・外す操作。お気に入りの操作も、`Like` の枠を共有するので、リアクションを外す。
  - 投稿に付いたリアクションを、絵文字で絞って付けた人の一覧として引くページ付きの操作。
- Web: 投稿の操作列にリアクションの選択画面（Unicode とローカルのカスタム絵文字）を置き、投稿の下に集計を並べる。Unicode とローカルのカスタム絵文字の集計は、押すと同じ絵文字で加わる・外すことができる。リモートのカスタム絵文字の集計は表示だけ。設計は [web-client.md](../architecture/web-client/README.md) に従う。

### 見直す条件

- 主要実装のどれかが絵文字の `id` の形式やローカル限定の属性を前提にし始めたとき、または Misskey とのリアクションの往復でライセンスやロールの制限が必要だと分かったとき。
- Mastodon が `content` 付きの `Like` をお気に入り以外として扱う、または Misskey / Akkoma が `content` 付きの `Like` のリアクションへの読み替えをやめる場合は、NodeInfo での送信の出し分けを検討し直す。
- ローカルの人にもお気に入りと複数のリアクションの同時付与が要るようになった場合は、資源を独立させる形を検討し、Mastodon と Misskey での状態の食い違いを受け入れるか、送信の出し分けと組み合わせる。
- 第三のサーバーの絵文字のリアクションが、記録が無くて捨てられる割合が運用で大きいと分かったとき（`Emoji` の `id` の取得を検討する）。
- FEP-c0e0 などで、リアクションの `Emoji` の真正性を確かめる仕組みが定まったとき。

## 経緯

M4 は、カスタム絵文字とリアクションが Mastodon / Misskey / Pleroma(Akkoma) と往復することを完了条件にする（[vision](../requirements/00-vision.md)、G4）。関連する要件は FR-111・FR-116・FR-006・FR-104・FR-112・FR-205・FR-405・FR-702・FR-704・FR-708・FR-720・FR-409 / NFR-COST-4。

カスタム絵文字は、2026-10-20 に各実装のソースと公式文書で確かめた。相互運用に効くのは `name`（`:shortcode:`）・`icon.url`・`updated` の 3 つで、どの実装も `id` の形式を前提にしない。Mastodon と Misskey は `(shortcode, ドメイン)` で絵文字を識別し、`updated` か画像の URL の変化で取り直す。`id` の形は Mastodon が `/emojis/{数値 ID}`、Misskey が `/emojis/{name}`、Akkoma は画像の URL や `null` のことがある。Mastodon は PNG / GIF / WebP の 256 KiB 未満で、GoToSocial は既定でローカル 50 KiB・リモート 100 KiB まで。受け手が `mediaType` を読まない実装（Misskey）もあるので、形式は中身で判定する。
出典: Mastodon の [`custom_emoji.rb`](https://github.com/mastodon/mastodon/blob/main/app/models/custom_emoji.rb)・[`emoji_serializer.rb`](https://github.com/mastodon/mastodon/blob/main/app/serializers/activitypub/emoji_serializer.rb)、Misskey の [`ApRendererService.ts`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/ApRendererService.ts)、GoToSocial の [Media の設定](https://docs.gotosocial.org/en/latest/configuration/media/)、Akkoma の [Custom Emoji](https://docs.akkoma.dev/stable/configuration/custom_emoji/)。

リアクションも同じ日に各実装の既定ブランチのソースで確かめた。Misskey は `Like` に `content` と `_misskey_reaction` を付けて送り、1 人 1 投稿に 1 つで、無い・解釈できない絵文字は ❤ にする（[ReactionService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/ReactionService.ts)）。Pleroma / Akkoma は `EmojiReact` を送り、絵文字ごとに複数を持ち、絵文字かショートコードを `content` に持つ `Like` を `EmojiReact` に書き換える（[transmogrifier.ex](https://akkoma.dev/AkkomaGang/akkoma/src/branch/develop/lib/pleroma/web/activity_pub/transmogrifier.ex)）。Mastodon と GoToSocial は `content` を見ず `Like` をお気に入りとして扱い、Mastodon は `EmojiReact` を捨てる（[like.rb](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/activity/like.rb)、[federatingdb/like.go](https://codeberg.org/superseriousbusiness/gotosocial/src/branch/main/internal/federation/federatingdb/like.go)）。[FEP-c0e0](https://codeberg.org/fediverse/fep/src/branch/main/fep/c0e0/fep-c0e0.md)（Draft）はこの慣行を文書にしている。そこで、Mastodon と Misskey で状態が食い違わない `Like` の枠と、1 つの送信の形を選んだ。

受信のカスタム絵文字は、当初は第三のサーバーの絵文字もその `Emoji` の `id` で識別して取り込むとしていた。しかしこれだと、ある人のリアクション 1 つで、別のサーバーの絵文字の見た目をこのサーバー全体で差し替えられる。そこで第三のサーバーの絵文字は記録済みのものだけを使う形にし、`content.RecordRemoteReaction` と `testkit.RemoteInteractionContract` と [federation.md §3.4](../architecture/federation.md#34-アクティビティごとのハンドラ) をこの規則にそろえた（#1469）。

作成の権限は、M4.2 の課金のプランに結び付くロールのために ADR-0070 が分け、`created_by_user_id` を足した。

## 採らなかった案

- Misskey 相当（ライセンス・センシティブ・ローカル限定・ロールでの利用の制限）: Misskey の利用者に馴染みのある細かい制御ができるが、`_misskey_license` 以外は連合せず相互運用に効かない。ロールの制限はリアクションと権限に波及する。
- ファイルの絵文字パック（Akkoma 方式）: 絵文字を Git で管理できるが、コンテナや Cloud Run の環境で運用者がファイルを置きにくく、管理画面で完結しない。
- `id` を `https://{host}/emojis/{shortcode}` にする（Misskey 方式）: 読みやすいが、削除して同じ shortcode で作り直すと別の絵文字が同じ `id` になり、ADR-0018 の「`id` は UUIDv7 だけで、名前を含めない」にも反する。
- `id` を送らない、または画像の URL を `id` にする: 単純だが、Mastodon の `GET` での解決と Misskey の `uri` の記録が使えない。
- お気に入りに絵文字を持たせる（`favourites` の拡張）: Mastodon・Misskey とは状態が合うが、Akkoma の複数のリアクションが 1 つに潰れてお気に入りとも衝突し、主キーと意味も変わる。
- リアクションとお気に入りを独立させ、ローカルの人も複数付けられるようにする: Akkoma と同じ表現力を持てるが、2 つの `Like` が Mastodon と Misskey で 1 つに潰れ、片方の `Undo` で両方消える。
- NodeInfo の `software.name` で送信を出し分ける（Pleroma / Akkoma 系に `EmojiReact`）: Akkoma に正しい型で届くが、NodeInfo の取得の失敗やフォーク（Firefish 系・Sharkey・Iceshrimp など）の判定の誤りがそのまま食い違いになる。
- 第三のサーバーの絵文字をリアクションの `Emoji` で取り込む: すべて受け取れるが、送り手が他のサーバーの絵文字の行と画像を決められ、§1 の識別の規則と両立しない。
- 第三のサーバーの絵文字を送り手のドメインの行として記録する: 偽装を防げるが、同じ絵文字が送り手ごとに別の行になって集計が割れ、送り手が名乗った shortcode で自分のドメインの行を上書きできる。
- 第三のサーバーの絵文字の `Emoji` の `id` を取得して確かめる: 忠実さが一番高いが、リアクション 1 つごとに外への取得が増え、取得のジョブ・失敗の扱い・SSRF の経路の検査が要る。Akkoma の利用者は多く自分のサーバーの絵文字を使い、その作者の投稿やプロフィールを一度受け取っていれば記録できる。

## Consequences（影響）

- 良い影響
  - Mastodon・Misskey・GoToSocial・Akkoma の絵文字を投稿とプロフィールで表示でき、こちらの絵文字も相手で表示される。
  - 投稿・プロフィール・リアクションが、絵文字の行・`Emoji` タグ・API の `CustomEmoji` を共有する。
  - 送信の形が 1 つで、相手のソフトウェアの判定に依存しない。Mastodon と GoToSocial の人には、リアクションがお気に入りとして見える。
  - ローカルの人の状態が、Mastodon・Misskey・Akkoma のどれとも 1 対 1 に対応し、取り消しで食い違わない。Akkoma の人の複数のリアクションとお気に入りは失わずに記録する。
  - リアクションの送り手は、他のサーバーの絵文字の行と画像を作れず、変えられない。
- 悪い影響・トレードオフ
  - 削除した絵文字は、過去の投稿でも文字に戻る。連合先が複製した画像は消せない（Mastodon と同じ）。
  - ライセンス・センシティブ・ロールの制限を持たないので、Misskey から届く `_misskey_license` は保存しない。
  - APNG は静止画として扱うので、ローカルの APNG の絵文字は動かない。GIF（とアニメーション WebP）で登録する。
  - 原本を保存しないので、Misskey などから届く大きな絵文字も、ローカルへ複製すると 256×256 の箱に縮む。
  - shortcode を大文字と小文字で区別するので、`Blob` と `blob` を別の絵文字として登録できる（Mastodon と同じ）。
  - ローカルの人は 1 投稿に複数のリアクションを付けられず、お気に入りとリアクションも同時に付けられない。リモートのカスタム絵文字のリアクションに同じ絵文字で加われない。
  - Misskey の人のリアクションはリアクションとして数えるので、`favourites_count` は Mastodon の数え方（Misskey のリアクションを含む）とずれる。
  - Pleroma / Akkoma にはリアクションが `EmojiReact` でなく `Like` として届くので、`Like` から読み替えられたリアクションとして記録される（表示は同じ）。
  - `Status` の集計は 20 種類までで、それより少ない件数の絵文字は付けた人の一覧でだけ見える。
  - Unicode の `emoji-test.txt` から作る正規化の表を、Unicode のバージョンに合わせて更新する手間がある。
  - このサーバーがまだ見たことのない第三のサーバーの絵文字のリアクションは捨てるので、Akkoma の人のリアクションの一部が件数に出ない。記録済みの第三のサーバーの絵文字の画像が古くても、リアクションでは更新しない（そのサーバーの文書が届いたときに更新される）。
- 後続で必要になる作業
  - Mastodon の絵文字パック（`tootctl emoji import` の tar.gz）の取り込みは別の Issue で検討する。

## Open Questions / 要検証

- 要検証: GoToSocial の絵文字の `id` の形・shortcode の上限・受信の識別、Akkoma の受信の識別と `updated` の扱い、Misskey のローカルの絵文字の形式とサイズの制限。
- 要検証: Pleroma 本家（`git.pleroma.social`）の受信が Akkoma と同じく `content` 付きの `Like` を `EmojiReact` に読み替えるか。Firefish 系・Sharkey・Iceshrimp.NET の受信が Misskey と同じか。
- 要検証: Misskey が `tag` の `Emoji` の `name` をコロン付きで送るか（`misskey/circa-2022/like-reaction.json` はコロン付き）、今の版でも同じか。
- 要検証: Akkoma の利用者のリアクションのうち、第三のサーバーの絵文字の割合（運用の指標が無いので、捨てた件数をログから見積もる）。
