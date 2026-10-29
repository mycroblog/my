## 1. 共通規約

### 1.1 テーブルの分類

すべてのテーブルに、次のいずれかの分類を付けます（ADR-0020 規約 1, 5）。各表の見出しの横に【分類】を記します。

| 分類 | 書き込み | 説明 |
| --- | --- | --- |
| 【リソース】 | INSERT のみ（消去ジョブによる物理削除を除く） | 識別子を持つ「もの」。作成時に決まる不変の事実だけを持つ |
| 【イベント】 | INSERT のみ | 起きた事実。**発生日時のカラムは 1 つだけ**（`*_at`）。訂正・取り消しは別のイベントで表す |
| 【投影】 | 所有モジュールの projector だけが UPSERT / DELETE | イベントから導出した現在値。§9 の再構築手順で作り直せる |
| 【現在値】 | 所有モジュールが UPSERT / DELETE | 履歴に意味がない高頻度の関係・設定値（ADR-0020 規約 5）。お気に入り・ミュート・利用者設定など |
| 【基盤】 | アダプタ | ジョブ・KV |

- 版（リビジョン）は【イベント】の一種です。「最新版」を頻繁に引くものは投影（`*_heads`）で指します。最新版を引く頻度が低いものは `(親, id DESC)` のインデックスで直接引きます。
- 期限（`expires_at`、`closes_at`）は「決定の内容」であって発生日時ではありません。イベントが持ってもよい属性です。
- 例外として、`kernel.Sealer` で封をした列（`*_enc`、`sealed_*`）は【リソース】【イベント】でも、KEK のローテーションのときに `my kek rewrap`（`internal/app/rewrap`）が UPDATE します（ADR-0019 §6 の「順次再暗号化」）。
  書き換えるのは封の中の DEK を包み直した部分だけで、暗号化された秘密（平文）は変わらないため、記録した事実は変わりません。行がまだ読んだ値を持つときだけ置き換えるので、モジュールの書き込みとは競合しません。

### 1.2 論理型

| 論理型 | 意味 | SQLite 系 | PostgreSQL 系 |
| --- | --- | --- | --- |
| `id` | UUIDv7 の正規形文字列（36 文字、小文字） | `TEXT` | `uuid`（16 バイトのバイト順 = 時刻順。[ADR-0078](../../adr/0078-postgres-uuid-columns.md)）。Go では `string`。UUID でない値を持つ列（外部の事業者の ID、`client_id`、監査ログの `target_id`、`emoji_key` など）は `text COLLATE "C"` |
| `string(n)` | 長さ上限つき文字列（上限はアプリで検証） | `TEXT` | `text` |
| `text` | 長文 | `TEXT` | `text` |
| `iri` | URL / ActivityPub の IRI（最大 2048 文字を想定） | `TEXT` | `text` |
| `int` / `bigint` | 整数 | `INTEGER` | `integer` / `bigint` |
| `real` | 倍精度の浮動小数点数（メディアの焦点など、誤差を許す量だけ） | `REAL` | `double precision` |
| `bool` | 真偽値 | `INTEGER`（0/1） | `boolean` |
| `timestamp` | UTC・ミリ秒精度の時刻 | `INTEGER`（Unix ミリ秒、#17 の推奨） | `timestamptz` |
| `date` | 時刻とタイムゾーンを持たない暦の日付（`YYYY-MM-DD`、0001-01-01〜9999-12-31） | `TEXT` + `CHECK (date(x) IS x)` | `date` |
| `lang` | BCP 47 言語タグ。不明は `und` | `TEXT` | `text` |
| `enum(…)` | 英小文字スネークケースの列挙 | `TEXT` + `CHECK` | `text` + `CHECK` |
| `bytes` | バイナリ（暗号化済みの秘密など） | `BLOB` | `bytea` |
| `document` | **解釈しない** JSON 文書のバイト列（受信した AP 文書の原本など）。書いたバイト列のまま読み戻す。検索・制約・部分更新に使わない | `BLOB` | `bytea` |

- `document` を JSON の型（PostgreSQL の `jsonb`・`json`）や `text` にしないのは、データベースに文書を解釈させないためです。
  - `jsonb` はキーの順序と空白を変え、重複したキーは最後の値だけを残します。AS2 の正規化（[federation §4](../federation.md#4-as2-の正規化)）は最初の値を読み、`contentMap` の順序にも意味を持たせるので、方言によって処理の結果が変わってしまいます。
  - `jsonb` は `\u0000` と対になっていないサロゲートのエスケープを、`text` はヌル文字を受け付けません。そのような受信を `500` で返すと、送信側は再送し続けます。
  - 受信した文書の原本は、障害調査とフィクスチャの採取のため、受け取ったバイト列のまま残します。
- `date` は支払日のように、カードの明細や請求書に書かれた日付そのものです（[ADR-0067 §5](../../adr/0067-finance-cost-records.md)）。時刻の値ではないので、`timestamp` の「UTC で保持」は当たりません。
  SQLite の `date()` は実在する `YYYY-MM-DD` だけを同じ文字列で返すので、`CHECK` は `2026-02-30` や時刻つきの値を拒みます。`TEXT` のままの比較と並べ替えは日付の順と一致します。
  PostgreSQL の `date` はテストの pgmem（[ADR-0041](../../adr/0041-postgres-tests-on-pgmem.md)）でも実際の PostgreSQL と同じに動き、`testkit.FinanceRepositoryContract` が両方言で確かめます。
  Go では `finance.Date`（`internal/domain/finance`）で持ち、方言のアダプタが `YYYY-MM-DD` の文字列と `pgtype.Date` に変えます。
- 文字列の論理型（`id`・`string(n)`・`text`・`iri`・`lang`・`enum`）の値は、妥当な UTF-8 でヌル文字（U+0000）を含みません。
  PostgreSQL の `text` はヌル文字を保存できず、SQLite は保存できるので、方言で結果が変わらないようにアプリが書く前に守ります。
  ローカルの入力（クライアント API・CLI・認証画面から文字列の列に書く値）は、モジュールのドメインの検証で拒否します。
  検証は入力の種類ごとに 1 か所にあり（`content.ValidText`、`moderation.TextError` を返す処分と解除の検証、`identity` の `Validate…` など）、クライアント API はそれを `422` で返します。
  JSON の本文は不正な UTF-8 を U+FFFD として読みますが、`multipart/form-data` の部分とヘッダー（`Idempotency-Key`）はバイト列のまま届くので、ヌル文字と不正な UTF-8 の両方を検証します。
  リモートの文書の文字列は正規化でヌル文字を U+FFFD に置き換えます（[federation §4](../federation.md#4-as2-の正規化)）。
  検索に使う入力も同じです。PostgreSQL はそのようなパラメーターで検索に失敗し（SQLSTATE 22021）、SQLite は何も見つけないので、検索する前に HTTP の入口で拒否します。
  リクエストのパス・クエリのパラメーター・`application/x-www-form-urlencoded` の本文のフィールドは `platform` の `rejectInvalidText` が `400` に、
  クライアント API の JSON の本文の文字列は `clientapi` の `rejectInvalidBodyText`（strict のミドルウェア）がプロパティ名を示す `422` にします。
  検索に使うヘッダーと Cookie のトークン（アクセストークン・セッション）は、ハッシュにしてから検索します。
- 表記: カラムは特記がなければ `NOT NULL`。NULL 可のものは「NULL 可」と書きます。
  - **状態を NULL で表すことはしません**（ADR-0020 規約 4）。NULL 可は「任意の関連」（例: 通報に基づかない処分の `report_id`）と「無期限」（`expires_at`）に限ります。
- 名前: カラム・テーブルは英語スネークケースで、テーブル名は複数形です。
  - 大文字小文字を無視する値は正規化済みカラム（`*_normalized`）を別に持ち、そちらに一意制約を張ります。
  - ドメイン名とリモートの URI のホストは、IDNA 2008 の A-label（小文字の punycode）で保存します（[federation §1.2](../federation.md#12-自サーバーの-uri)）。
- 「ローカル」を表すドメインは空文字 `''` です。NULL は一意制約上は互いに別の値として扱われるため、NULL では表しません。

### 1.3 ID とキーセットページング

- 主キーは UUIDv7 です（`IdGenerator` ポート、`uuid.NewV7()` と同じくミリ秒の後の 12 ビットにミリ秒未満の時刻を入れる RFC 9562 の方式）。辞書順と時刻順が一致するので、`ORDER BY id` がそのまま時系列になります。
  - 1 つのプロセスが発行する ID は、システム時計が 1 分以内だけ戻っても増え続けます（`Clock` と同じく、戻った間は最後の時刻の次の値を使います）。`uuid.NewV7()` は秒をまたいで戻った時計にそのまま従うので、時刻同期が数秒おきに時計を戻す仮想マシンでは、続けて行った操作の ID の順序が逆になります。
- PostgreSQL の `uuid` には `max` / `min` の集約関数が無いので、「最新の版の `id`」は `ORDER BY id DESC LIMIT 1` や `DISTINCT ON` で取ります。
- ページングは常にキーセット方式（`id < :cursor ORDER BY id DESC LIMIT :n + 1`）です。不透明なカーソルの中身はこの `id` です（ADR-0016）。
- リモート投稿の `id` は AP の `published` から生成します（未来の時刻は現在時刻に丸めます）。Go では `IDGenerator.NewIDAt` です。
  - 同じミリ秒の ID どうしの順序は保証せず、ランダム部で一意性だけを確保します。キーセットページングは `id` の全順序で進むので、順序が任意でも行の取りこぼしや重複は起きません。
- UUIDv7 は時刻から下限値を作れるので、`id >= uuidv7_floor(now - N days)` を読み取り量の上限として使えます（§6）。Go では `kernel.UUIDv7Floor` が下限値を返します。

### 1.4 参照と削除

- 参照は `xxx_id` による論理参照です。物理的な外部キーは同一モジュール内に限り任意で、モジュールをまたぐ参照には付けません。
  - Aurora DSQL も外部キーに対応した（#18）ため、同一モジュール内の外部キーは方言別スキーマで付ける方向で検討します（§11）。
- 削除の扱い（ADR-0020 規約 6）:
  - **論理的な削除は削除イベントで表します**（`status_deletions`、`account_lifecycle_events`）。
  - **物理削除は消去ジョブだけが行います**。個人データの行（本文・プロフィール・メールアドレス）を消し、投影を更新します。
  - 連合の重複排除に必要な URI は削除イベント側に残ります（tombstone を兼ねます）。
