# ADR-0052: 絵文字リアクション（Misskey / Pleroma / Akkoma 互換）

- Status: Accepted（§1 の「絵文字の行は ADR-0051 の規則で `custom_emojis` に取り込みます（第三のサーバーの絵文字は、その `Emoji` の `id` で識別する）」の文を ADR-0104（リアクションの第三のサーバーのカスタム絵文字は記録済みのものだけを使う）が置き換える）
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #619

## Context（背景・課題）

M4 の完了条件は「カスタム絵文字とリアクションが Mastodon / Misskey / Pleroma(Akkoma) と往復する」ことです（[00-vision.md](../requirements/00-vision.md)、G4）。
しかし [01-functional.md](../requirements/01-functional.md) に絵文字リアクションの要件はなく、FR-107 はお気に入り（`Like`）だけ、FR-205 の通知の種類にもリアクションはありません。

main のコードは次のとおりです。

- 受信: `internal/domain/federation/normalize.go` が、`EmojiReact` の `content` と、`Like` の `_misskey_reaction` を `Activity.Reaction` に、`tag` の `Emoji` を `Activity.Emojis` に正規化します。
- `internal/app/federation/interactions.go` は `Like` と `EmojiReact` を同じハンドラで受け、`content.RecordRemoteFavourite` でお気に入りとして記録します。絵文字は捨てます（[federation.md §3.4](../architecture/federation.md#34-アクティビティごとのハンドラ)）。
- `favourites` は主キー `(account_id, status_id)` の現在値の表で、1 人が 1 投稿に 1 つです。送信は、リモートの投稿へのお気に入りを、作者の inbox への `Like` / `Undo Like` にします。
- 絵文字の保存・集計・クライアント API・Web の UI・送信はありません。フィクスチャは `pkg/testkit/fixtures/federation/akkoma/3.20/emoji-react*.json`（Unicode・送り手のサーバーのカスタム絵文字・第三のサーバーのカスタム絵文字）と `misskey/*/like-reaction.json` です。

主要実装の振る舞いは次のとおりです（2026-10-20 に各実装の既定ブランチのソースで確認）。

| 実装 | 送信 | 受信 | 1 人 1 投稿あたり |
| --- | --- | --- | --- |
| Misskey | `Like` に `content` と `_misskey_reaction`（同じ文字列）。カスタム絵文字ならローカルの絵文字の `Emoji` を `tag` に 1 つ（[ApRendererService.ts `renderLike`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/ApRendererService.ts)） | `Like` / `EmojiReact` / `EmojiReaction` を同じく扱い、`_misskey_reaction ?? content ?? name` を絵文字にする。無い・解釈できない絵文字は ❤ にする（[type.ts `isLike`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/type.ts)、[ApInboxService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/activitypub/ApInboxService.ts)、[ReactionService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/ReactionService.ts)）。リモートの人のカスタム絵文字はその人のホストの絵文字として引く | 1 つ。別の絵文字が来たら置き換え、`Undo` は絵文字に関係なくその人のリアクションを消す |
| Pleroma / Akkoma | `EmojiReact`（`content` は絵文字、カスタム絵文字は `tag` の `Emoji`。`name` はコロンなし。第三のサーバーの絵文字も使える） | `EmojiReact`。`content` が絵文字かショートコードの `Like` は `EmojiReact` に書き換え、それ以外の `Like` はお気に入り（[transmogrifier.ex](https://akkoma.dev/AkkomaGang/akkoma/src/branch/develop/lib/pleroma/web/activity_pub/transmogrifier.ex)、[emoji_react_validator.ex](https://akkoma.dev/AkkomaGang/akkoma/src/branch/develop/lib/pleroma/web/activity_pub/object_validators/emoji_react_validator.ex)） | 絵文字ごとに 1 つ（複数可）。お気に入りとは別 |
| Mastodon | 送らない | `Like` だけを扱い、`content` / `_misskey_reaction` は見ない。`EmojiReact` は未知の型として捨てる。`Undo Like` は `Like` の `id` でなく主体と投稿の組でお気に入りを消す（[like.rb](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/activity/like.rb)、[undo.rb](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/activity/undo.rb)、[activity.rb](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/activity.rb)） | お気に入り 1 つ |
| GoToSocial | 送らない | `Like` をお気に入りとして扱い、`content` は見ない（[federatingdb/like.go](https://codeberg.org/superseriousbusiness/gotosocial/src/branch/main/internal/federation/federatingdb/like.go)） | お気に入り 1 つ |

[FEP-c0e0](https://codeberg.org/fediverse/fep/src/branch/main/fep/c0e0/fep-c0e0.md)（Draft）は、この慣行を `EmojiReact` として文書にしています。
`content` は 1 書記素の Unicode 絵文字かカスタム絵文字のショートコード、カスタム絵文字なら `tag` に `Emoji` を 1 つ（`name` はコロンで囲むのが推奨）、取り消しは `Undo`、送り手は `EmojiReact` の代わりに `content` 付きの `Like` を使ってよく、受け手は `content` 付きの `Like` を `EmojiReact` と同じく扱う、と定めます。
1 人が 1 投稿に付けられる数は実装に任せています。

カスタム絵文字そのもの（登録・リモートの絵文字の取り込み・`Emoji` タグの形）は ADR-0051 カスタム絵文字（#618）が決めます。この ADR はそれを前提に、リアクションの資源・連合の形・API・通知を決め、要件 FR-116（絵文字リアクション）と FR-205・FR-405・FR-708 の拡張の根拠になります。

## Decision Drivers（判断基準）

1. **相互運用（G4）**: Misskey と Akkoma ではリアクションとして、Mastodon と GoToSocial ではお気に入りとして届き、取り消しも全員で一致すること。
2. **ワイヤ上の一貫性**: 1 人が 1 投稿について相手のサーバーに残す状態が、相手の実装で矛盾しないこと（Mastodon と Misskey は 1 人 1 投稿に `Like` 1 つしか持てない）。
3. **受信の忠実さ**: Akkoma の人が付けた複数のリアクションとお気に入りを失わずに記録できること。
4. **データモデルの規律（ADR-0020）と既存の規則の再利用**: 閲覧の可否（単一のポリシー関数、ADR-0017）・ブロック・交流の禁止（FR-708）を、お気に入りと同じ経路で効かせること。
5. **実装量**: 相手のソフトウェアによる送信の出し分けなど、壊れやすい分岐を増やさないこと。

## Considered Options（選択肢）

### 資源の形

1. **A: お気に入りに絵文字を持たせる** — `favourites` に絵文字を足し、絵文字の無い行をお気に入り、ある行をリアクションとする。1 人 1 投稿に 1 つ。
2. **B: リアクションをお気に入りと別の資源にし、互いに独立させる** — 新しい `reactions` を作り、ローカルの人もお気に入りと複数のリアクションを同時に付けられる（Akkoma と同じ）。
3. **C: リアクションを別の資源にし、ローカルの人の「`Like` の枠」を 1 投稿に 1 つにする** — 記録は B と同じく `reactions` に分ける（リモートの人の複数のリアクションも残す）。ローカルの人は 1 投稿にお気に入りかリアクション 1 つのどちらか一方だけを付け、付け替えは置き換えにする（Misskey と同じ）。

### 送信の形

1. **X: 常に `content` と `_misskey_reaction` 付きの `Like` を送る** — 相手のソフトウェアを見ない。
2. **Y: NodeInfo の `software.name` で出し分ける** — Pleroma / Akkoma 系には `EmojiReact`、Misskey 系には `_misskey_reaction` 付きの `Like`、Mastodon / GoToSocial には `Like`（またはリアクションを送らない）。

## Decision（決定）

**資源は C、送信は X にします。**

### 1. 資源とお気に入りとの関係

- リアクションは `content` モジュールのリソース `reactions` にします（ADR-0020）。行は 1 つのリアクションで、主体・投稿・絵文字・`uri`（`Like` / `EmojiReact` の `id`）・作成時刻を持ちます。一意制約は `(account_id, status_id, 絵文字)` と `(uri)` です。
  絵文字は「Unicode 絵文字の文字列」か「`custom_emojis` の行」のどちらかで、集計に使う正規化した鍵（Unicode はその文字列、カスタム絵文字は `custom_emojis.id`）を持ちます。
- 取り消しは行の削除と `content.unreacted` イベントです（お気に入りと同じく、取り消した後の付け直しは新しい行）。絵文字ごとの件数は `content` が持つ投影（例: `status_reaction_counts (status_id, 絵文字の鍵, count)`）で、行を実際に挿入・削除したときだけ増減します。表と列の名前は実装の issue で [data-model.md](../architecture/data-model/README.md) に書きます。
- `favourites` と `favourites_count` は今のまま残します。お気に入りとリアクションは別の資源で、API でも別に数えます。
- **ローカルの人**は 1 投稿にお気に入りかリアクション 1 つのどちらか一方だけを持ちます（`Like` の枠）。リアクションを付けると、同じ投稿へのお気に入りや別の絵文字のリアクションを同じトランザクションで外し、外したものの `Undo` を送ってから新しい `Like` を送ります（どちらもアウトボックス経由）。お気に入りを付けたときも同じです。
  Mastodon は `Undo Like` で `id` に関係なくお気に入りを消し、Misskey は 1 人 1 投稿にリアクションを 1 つしか持たないので、枠を 2 つ使うと相手のサーバーでの状態が食い違います（判断基準 2）。
- **リモートの人**は、相手のサーバーが送ったとおりに記録します。Akkoma の人のお気に入りと絵文字ごとの複数のリアクションは、`favourites` と `reactions` の別々の行になります。
- 受信の振り分けは次のとおりです。

| 受信 | 記録 |
| --- | --- |
| `EmojiReact`（`content` が絵文字） | リアクション |
| `Like` で `_misskey_reaction` か `content` が絵文字（FEP-c0e0、Akkoma と同じ判定） | リアクション |
| それ以外の `Like` | お気に入り（今と同じ） |
| `Undo` | 対象の `uri` で `reactions`、`favourites` の順に引いて消す。どちらにも無ければ、埋め込みの `object` を上の表で振り分け、リアクションならその主体のその投稿への同じ絵文字のリアクションを、お気に入りならその主体のお気に入りを消す |

- 受け入れる絵文字は次のとおりです。外れた `EmojiReact` は捨て、外れた `Like` は Misskey と同じくお気に入りとして記録します（Mastodon の利用者から見た件数と合わせる）。
  - Unicode 絵文字: 1 書記素の絵文字（FEP-c0e0）。異体字セレクタの有無などの揺れは、Unicode の `emoji-test.txt` から生成した表で fully-qualified の形に正規化して集計を揃えます（Pleroma も同じ方向で正規化する。[MR !3708](https://git.pleroma.social/pleroma/pleroma/-/merge_requests/3708)）。表はリポジトリ内で生成し、外部ライブラリを足しません。
  - カスタム絵文字: `content` が `:shortcode:` で、`tag` に `name` が一致する（コロンの有無を問わない。Akkoma はコロンなし、Misskey と Mastodon はコロン付き）`Emoji` が 1 つあるもの。絵文字の行は ADR-0051 の規則で `custom_emojis` に取り込みます（第三のサーバーの絵文字は、その `Emoji` の `id` で識別する）。Iceshrimp.NET 系の `name@host` の接尾辞は落とします（Akkoma と同じ）。

### 2. ローカルの人が使える絵文字

- Unicode 絵文字と、このサーバーで有効なローカルのカスタム絵文字（ADR-0051 の `custom_emoji_settings.enabled`）。
- リモートのカスタム絵文字ではリアクションできません。その投稿にすでに付いているリアクションに同じ絵文字で加わることもしません。Misskey は送り手のホスト以外の絵文字を ❤ に置き換えるので、送っても相手に同じ絵文字として届かないためです。

### 3. 送信の形（X）

- リアクションは、FEP-c0e0 の互換の形として、**作者の inbox へ `content` と `_misskey_reaction` を付けた `Like`** を送ります。宛先・署名・配送は今の `Like` と同じです（[federation.md §4](../architecture/federation.md)）。
- `content` と `_misskey_reaction` は同じ文字列で、Unicode ならその絵文字、カスタム絵文字なら `:shortcode:` です。カスタム絵文字は `tag` に `Emoji` を 1 つ付けます。形は投稿の `tag` と同じ Mastodon の `toot:Emoji`（`id`・`type: "Emoji"`・コロンで囲んだ `name`・`updated`・`icon` の `Image`（`mediaType` と `url`））で、ADR-0051 が決める形に従います。`@context` には `toot` と `misskey`（`https://misskey-hub.net/ns#`）の `_misskey_reaction` を足します。
- 取り消しは `Undo`（`object` に元の `Like` を埋め込む）です。
- この 1 つの形で、Misskey（`_misskey_reaction`）と Pleroma / Akkoma（`content` 付きの `Like` を `EmojiReact` に書き換える）ではリアクション、Mastodon と GoToSocial ではお気に入りとして届きます。
- `EmojiReact` は送りません。受信は ADR-0006 の互換拡張に、LitePub の `EmojiReact` と Misskey の `_misskey_reaction` を加えます（ADR-0006 の他の部分は変えません）。投稿の `emojiReactions` コレクション（Fedibird の拡張）は公開しません。
- `local` の投稿（ADR-0017）とローカルの人の投稿へのリアクションは連合しません（お気に入りと同じく、リモートの投稿へのものだけを送る）。

### 4. 公開範囲・ブロック・モデレーション

- リアクションを付けられる投稿は、お気に入りと同じ単一のポリシー関数（`canView`。リモートの人は `ContextObjectFetch`）で閲覧できる、削除されていない投稿です。ブロックしている・されている相手の投稿には付けられず、受信したものも捨てます。
- 交流の禁止（FR-708、`account_restrictions` の `interact`）はリアクションも禁止します。
- 投稿の削除・アカウントの削除・凍結での消し方は `favourites` と同じ扱いにします（[data-model.md](../architecture/data-model/README.md) のアカウント削除の表に足す）。

### 5. 通知・クライアント API・Web

- 通知の種類に `reaction` を足します（FR-205）。お気に入りと同じく、ミュート・ブロック・通知の除外の規則に従います。同じ人が別の絵文字で付け直したときは、新しい通知を作ります。
- クライアント API（ADR-0016、パスと名前は [client-api.md](../architecture/client-api.md) の規約で実装時に決める）:
  - `Status` に、絵文字ごとの件数と自分が付けたかどうかの集計（件数の多い順に最大 20 種類）と、カスタム絵文字の表示に要る情報（ショートコード・画像の URL・ドメイン）を足す。
  - 自分のリアクションを付ける（置き換える）・外す操作。お気に入りの操作も、`Like` の枠を共有するので、リアクションを外す。
  - 投稿に付いたリアクションを、絵文字で絞って付けた人の一覧として引くページ付きの操作。
- Web: 投稿の操作列にリアクションの選択画面（Unicode とローカルのカスタム絵文字）を足し、投稿の下に集計を並べます。Unicode とローカルのカスタム絵文字の集計は、押すと同じ絵文字で加わる・外すことができます。リモートのカスタム絵文字の集計は表示だけです。設計は [web-client.md](../architecture/web-client/README.md) に従います。

### この決定を見直す条件

- Mastodon が `content` 付きの `Like` をお気に入り以外として扱う、または Misskey / Akkoma が `content` 付きの `Like` のリアクションへの読み替えをやめる場合は、Y（出し分け）を検討し直します。
- ローカルの人にもお気に入りと複数のリアクションの同時付与が要るようになった場合は B を検討し、Mastodon と Misskey での状態の食い違いを受け入れるか、Y と組み合わせます。

## Consequences（影響）

- 良い影響
  - 送信の形が 1 つで、相手のソフトウェアの判定に依存しません。Mastodon と GoToSocial の人には、リアクションがお気に入りとして見えます。
  - ローカルの人の状態が、Mastodon・Misskey・Akkoma のどれとも 1 対 1 に対応し、取り消しで食い違いません。
  - Akkoma の人の複数のリアクションとお気に入りを失わずに記録し、集計に出せます。
  - 閲覧の可否・ブロック・交流の禁止は、お気に入りと同じ経路を通ります。
- 悪い影響・トレードオフ
  - ローカルの人は Akkoma の UI のように 1 投稿に複数のリアクションを付けられず、お気に入りとリアクションも同時に付けられません。
  - Misskey の人のリアクションは、今はお気に入りとして数えていますが、リアクションとして数えるようになります。`favourites_count` は Mastodon の数え方（Misskey のリアクションを含む）とずれます。
  - Pleroma / Akkoma にはリアクションが `EmojiReact` でなく `Like` として届くので、Akkoma 側で `Like` から読み替えられたリアクションとして記録されます（表示は同じ。要検証: Pleroma 本家の `transmogrifier.ex` も同じ読み替えをするか）。
  - ローカルの人は、リモートのカスタム絵文字のリアクションに同じ絵文字で加われません。
  - `Status` の集計は 20 種類までで、それより少ない件数の絵文字は付けた人の一覧でだけ見えます。
  - Unicode の `emoji-test.txt` から作る正規化の表を、Unicode のバージョンに合わせて更新する手間が増えます。
- 後続で必要になる作業（Issue 化する）
  - 受信: `reactions` と件数の投影のスキーマ（`schema/*.sql`）、`content` の記録・取り消しのコマンドとイベント、`federation` の受信の振り分けと `Undo` の照合、フィクスチャのコントラクトテスト（`testkit.RemoteInteractionContract` にリアクションを足す）、[federation.md §3.4](../architecture/federation.md#34-アクティビティごとのハンドラ) と data-model.md の更新。
  - 送信と `Like` の枠: ローカルのコマンド（付け替え・置き換えの `Undo`）、`Like` の本文の組み立て、ライブ相互運用テスト（`make federation-live`）に Misskey とのリアクションの往復を足す。
  - 通知 `reaction`（FR-205）と Web Push の文言（`locales/en`・`locales/ja`）。
  - クライアント API（`api/openapi.yaml`）と Web の UI（選択画面・集計・付けた人の一覧）。
  - 交流の禁止（FR-708）とアカウント削除・凍結の消し方の更新。

## Pros and Cons of the Options（詳細比較・任意）

| | A: お気に入りに絵文字 | B: 別資源・独立 | C: 別資源・ローカルは枠 1 つ（採用） |
| --- | --- | --- | --- |
| Mastodon / Misskey との状態の一致 | ○ | ✕（お気に入りとリアクションの 2 つの `Like` が 1 つに潰れ、片方の `Undo` で両方消える） | ○ |
| Akkoma の複数のリアクションの受信 | ✕（1 つに潰れ、お気に入りとも衝突する） | ○ | ○ |
| ローカルの人の表現力 | 1 つ | 複数 + お気に入り | 1 つ（Misskey と同じ） |
| 既存の `favourites` への影響 | 主キーと意味が変わる | なし | なし（付け替えの処理が増える） |

B の一番の利点は、Akkoma と同じ表現力をローカルの人に与えられることです。Y と組み合わせれば Akkoma にはその表現力のまま届きますが、Mastodon と Misskey にはどの `Like` を送るかを相手ごとに選ぶ必要があり、NodeInfo の取得の失敗やフォーク（Firefish 系・Sharkey・Iceshrimp など）の判定の誤りがそのまま食い違いになります。

| | X: 常に `Like` + `_misskey_reaction` | Y: NodeInfo で出し分け |
| --- | --- | --- |
| Mastodon / GoToSocial | お気に入りとして届く | 選び方次第 |
| Misskey 系 | リアクション | リアクション |
| Pleroma / Akkoma | リアクション（`Like` から読み替え） | リアクション（`EmojiReact`） |
| 実装と失敗の余地 | 小さい | NodeInfo の取得・キャッシュ・フォークの判定が要る |

## Open Questions / 要検証

- 要検証: Pleroma 本家（`git.pleroma.social`）の受信が Akkoma と同じく `content` 付きの `Like` を `EmojiReact` に読み替えるか。Firefish 系・Sharkey・Iceshrimp.NET の受信が Misskey と同じか。
- 要検証: Misskey が `tag` の `Emoji` の `name` をコロン付きで送るか（`misskey/circa-2022/like-reaction.json` はコロン付き）、今の版でも同じか。
