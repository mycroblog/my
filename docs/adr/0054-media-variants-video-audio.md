# ADR-0054: メディアの派生と動画・音声の扱い

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #644

## Context（背景・課題）

決定者は次を指示している（#644、原文）:
「アイコン・投稿画像などを Small・Medium・Large でリサイズして保存する仕組みを導入。また、この時 Animated GIF も停止版・動いている版で保存する仕組み。同時に音声・動画の取り扱いについても ADR を建てて方針を取り決める」。
S / M / L の 3 段で保存すること、アニメーション GIF の静止版と動く版を両方保存することは決定済みで、この ADR はその大きさ・形式・作る時機・保存の形・API と ActivityPub への出し方と、動画・音声の方針を決める。

main の現状:

- [ADR-0005](0005-blob-storage-and-media.md)（Accepted）が `MediaProcessor` の port を決め、既定のアダプタは純 Go の `media/native`（`internal/adapter/media/native`）。
  `internal/app/media/media.go` の `Processor` は `Inspect` と `Process` を持ち、`Process` は再エンコードでメタデータを落とした `Full`（面積 3840×2160 まで）と `Thumbnail`（面積 640×360 まで）と blurhash を返す。
- 出力は JPEG（不透明）/ PNG（透過）/ GIF。純 Go に WebP のエンコーダがないので WebP は出力しない。アニメーション GIF は縮小せず全フレームを再エンコードし、サムネイルは最初のフレーム。アニメーション WebP は拒否する。
- アイコン・ヘッダーも投稿画像と同じ `full` / `thumbnail` の 2 段で、表示には `full` を使う（[data-model.md](../architecture/data-model/README.md) のプロフィールの節）。タイムラインの 40 px 前後のアイコンにも 3840×2160 までの画像を配ることがある。
- `media_variants.kind` は `full` / `thumbnail`、Blob のキーは `media/{media_id}/{kind}.{ext}`。処理は upload のあとのジョブ `media.process`（[ADR-0004](0004-job-queue-abstraction.md)）で、結果はイベント `media_processing_results` と子の `media_variants`（[ADR-0020](0020-data-modeling-principles.md)）。
- 動画・音声のローカルの添付はない（FR-105 の S。#629 がこの ADR を待っている）。リモートの動画・音声は宣言された値を `remote_media` に記録し、メディアプロキシ（[federation.md §9](../architecture/federation.md#9-リモートメディア)）が `full` だけを中継する。プロキシは `Range` を扱わない。
- [ADR-0019 §8](0019-security-baseline.md) が形式の許可リスト（JPEG / PNG / GIF / WebP / AVIF / MP4 / WebM / MP3 / Ogg）、サイズ（画像 16 MiB、動画・音声 100 MiB）、寸法の上限、メタデータの除去を決めている。
- ADR-0005 の Consequences は「動画のトランスコードは v0 では行わない（サイズ・形式制限のみ）」としている。

関連する要件: FR-105（画像の添付、動画・音声は S）、FR-111（カスタム絵文字、ADR-0051 カスタム絵文字 #618 が並行して起案中）、FR-409 / NFR-COST-4（リモートメディアのキャッシュ）、FR-803（期限切れデータの掃除）、NFR-SEC-6（アップロードの安全性）、[ADR-0009](0009-internationalization.md) と [ADR-0027](0027-ui-quality-gates.md)（`prefers-reduced-motion` を含む UI の品質）。

## Decision Drivers（判断基準）

1. **`CGO_ENABLED=0` の単一バイナリ**（[ADR-0001](0001-server-language-and-runtime.md)）と、1 vCPU / 1 GB で動くこと（NFR-COST-3）。cgo・常駐の外部プロセスを既定の構成に要求しない。
2. **位置情報などのメタデータを公開しない**（NFR-SEC-6、ADR-0019 §8）。動画・音声も例外にしない。
3. **連合の相互運用**: Mastodon・Misskey・GoToSocial・Pleroma / Akkoma が、送った `attachment` / `icon` / `image` を表示・再生できる。
4. **配る量を減らす**（NFR-COST-4）: 表示する大きさに合った派生を配る。CDN で長くキャッシュできる不変のキーにする。
5. **アクセシビリティ**: 動くものは `prefers-reduced-motion` と利用者の設定で止められる（WCAG 2.2 SC 2.2.2）。
6. 実装と依存を小さく保つ。追加する依存は AGPL と両立し、リリースから 3 日を過ぎたもの。

## Considered Options（選択肢）

画像の派生の形式:

- **A1. JPEG / PNG / GIF のまま S / M / L を作る**（標準ライブラリだけ）
- **A2. S / M を WebP（純 Go のエンコーダ）、L を JPEG / PNG / GIF にする**
- **A3. WebP と AVIF を WASM 版の libwebp / libaom（wazero）で作る**

動画・音声の処理:

- **V1. トランスコードしない。純 Go で container を解析して検証し、メタデータを落として並べ直す（remux）。ポスターはクライアントが作る**
- **V2. 外部コマンドの `ffmpeg` で H.264 / AAC にトランスコードし、ポスターも作る**（Mastodon と同じ方針）
- **V3. WASM 版の ffmpeg（`codeberg.org/gruf/go-ffmpreg`、wazero）を埋め込んでトランスコードする**（GoToSocial と同じ方針）

リモートのメディア:

- **R1. 派生させない**（いまのまま `full` を中継するだけ）
- **R2. プロキシで静止画の S だけを必要なときに作る**
- **R3. 取り込み時に取得して S / M / L を作って保存する**

## Decision（決定）

**A2（S / M の WebP は spike で確かめてから。だめなら A1）+ V1 + R2 にする。**

### 1. 画像の派生の段と大きさ

派生は用途ごとに次の箱に作る。アイコンは**正方形に切り抜いて**から箱に縮め、切り抜く位置は焦点（`media_focus`、なければ中央）に合わせる。それ以外は**縦横比を保って収め**（切り抜かない。表示側が `object-fit: cover` と焦点で切る）。箱より小さい元画像は拡大しない。

| 用途 | S | M | L |
| --- | --- | --- | --- |
| 投稿の画像 | 面積 640×360 | 面積 1920×1080 | 面積 3840×2160 |
| アイコン | 96×96 | 192×192 | 400×400 |
| ヘッダー | 750×250 | 1500×500 | 3000×1000 |
| カスタム絵文字 | 64×64 | 128×128 | 256×256 |

- 投稿画像の S と L は main の `thumbnail` / `full` と同じ面積（Mastodon の `small` / `original` と同じ。[media_attachment.rb](https://github.com/mastodon/mastodon/blob/main/app/models/media_attachment.rb)）。M は 1080p の画面で全面に出す大きさ。
- アイコン L とヘッダー M は Mastodon の 400×400 / 1500×500 に合わせる。アイコンを正方形に切り抜くのも Mastodon と同じ（`400x400#`）（[avatar.rb](https://github.com/mastodon/mastodon/blob/main/app/models/concerns/account/avatar.rb)、[header.rb](https://github.com/mastodon/mastodon/blob/main/app/models/concerns/account/header.rb)。2026-10-20 確認）。S / M はタイムラインの 48 px と 96 px を 2 倍の解像度で埋める大きさ。
- カスタム絵文字の受け付けの上限と登録の手順は ADR-0051（#618）が決め、この表は派生の箱だけを決める。
- 用途ごとの箱は `media.Sizes` に持たせ、upload の API が用途（`attachment` / `avatar` / `header` / `emoji`）を受け取る。用途を変えて使うときは、その用途で処理し直す（新しい処理結果）。
- 原本は保存しない（いまと同じ。位置情報を残さないため）。L が保存する最大のもの。

### 2. 形式と画質

- **L**: JPEG（不透明、品質 85）/ PNG（透過）。連合先がどこでも表示できる形式にする（Driver 3）。
- **S / M**: WebP（lossy、品質 80、透過は alpha 付き）。純 Go のエンコーダ [`github.com/deepteams/webp`](https://pkg.go.dev/github.com/deepteams/webp)（MIT、cgo なし、lossy / alpha / アニメーションに対応と記載。2026-10-20 確認）を使う。
  **spike で画質・速度・メモリ・ファズ（壊れた出力を作らないこと）、アニメーション WebP の読み書き、主要ブラウザでの表示を確かめてから採用する**。採用する版は、その時点でリリースから 3 日を過ぎたもの。spike が通らなければ S / M も JPEG / PNG にし（A1）、他の決定は変えない。
  WebP は主要ブラウザすべてで表示できる（Safari 14 以降。要検証）。
- AVIF は出力しない。純 Go のエンコーダがなく、A3 の WASM は 1 枚あたりの CPU が重い（要検証）。入力としての AVIF / HEIC も `media/native` は受け付けない（Mastodon も安全のため止めている。[media_attachment.rb](https://github.com/mastodon/mastodon/blob/main/app/models/media_attachment.rb)）。ADR-0019 §8 の許可リストは上限で、アダプタはその一部だけを受け付けてよい。
- blurhash は S から作る（いまと同じ）。

### 3. アニメーション画像

- 入力: GIF とアニメーション WebP（WebP の decoder がアニメーションに対応すると spike で確かめられたら。だめならいまと同じく拒否）。APNG は既定の画像（最初のフレーム）だけを読む静止画として扱う（標準の `image/png` の挙動）。
- フレーム数と全フレームの画素数の上限は `media.Limits`（いまと同じ）。
- アニメーションの入力は、**静止版（最初のフレーム）の S / M / L** と **動く版の S / M / L** の 6 つを作る。
  - 静止版は §2 の静止画と同じ形式。
  - 動く版の L は GIF（連合先が表示できる）、S / M はアニメーション WebP（spike が通らなければ GIF）。縮小した GIF はフレームごとに元のパレットへ減色し直す。
- クライアントは `<picture>` の `<source media="(prefers-reduced-motion: no-preference)">` に動く版を、`<img>` に静止版を置く。利用者の設定「アニメーションを自動再生しない」が有効なら `<source>` を出さず、ホバー・フォーカス・タップで動く版に替える。
- 投稿の `gifv`（リモートの Mastodon が GIF を MP4 にしたもの）は動画として扱い、自動再生は同じ条件で止める。

### 4. 作る時機とデータモデル

- 派生はいまと同じく upload のあとのジョブ `media.process` で、1 回の処理で全部作る（ADR-0004。ジョブは冪等）。upload の応答は `processing` を返す。
- `media_variants`（イベント、`media_processing_results` の子）の `kind` を `size`（`small` / `medium` / `large`）と `motion`（`still` / `animated`）の 2 列に替え、主キーを `(result_id, size, motion)` にする（ADR-0020。v0 なので移行はしない、ADR-0012）。
  動画・音声のポスターも `motion = still` の派生として同じ表に入れる。
- Blob のキーは `media/{media_id}/{result_id}/{size}.{ext}` と `media/{media_id}/{result_id}/{size}-animated.{ext}`。処理し直すとキーが変わるので、配信は `Cache-Control: public, max-age=31536000, immutable` にできる（Driver 4）。古い処理結果の Blob は掃除のジョブが消す（FR-803）。
- 大きさや形式の設定を変えたときの作り直しは、管理者のコマンドで全ファイルの `media.process` を入れ直す（後続 Issue）。

### 5. クライアント API と ActivityPub

- クライアント API の `MediaVariants` は `small` / `medium` / `large`（静止版、必須）と、アニメーションのときだけの `animated`（`small` / `medium` / `large`）にする。各派生は `url` / `content_type` / `width` / `height` / `size` を持ち、クライアントは `srcset` の幅指定で選ぶ。
  アカウントの `avatar_url` / `header_url` も同じ形のオブジェクト（`avatar` / `header`）に替える（v0 の破壊的変更、ADR-0012）。
- ActivityPub の `attachment` の `Document` は、`url` に L（アニメーションなら動く版の L の GIF）、`mediaType`・`width`・`height`・`blurhash`・`focalPoint`・`name` をいまと同じく載せる。
  動画・音声はポスターの L を `icon`（`Image`）に、長さを `duration`（ISO 8601）に載せる。Mastodon も同じ形で送っている（[note_serializer.rb](https://github.com/mastodon/mastodon/blob/main/app/serializers/activitypub/note_serializer.rb) の `icon` と `duration`。2026-10-20 確認）。
  `url` を複数の `Link` の配列にはしない（受け側の対応が揃っていない。要検証）。
- Actor の `icon` はアイコンの L、`image` はヘッダーの L（アニメーションなら動く版）。静止版は受け側が作る（Mastodon は GIF から `static` を作る）。

### 6. 動画・音声（V1）

- **受け付ける形式**（中身で判定、ADR-0019 §8）:
  - 動画: MP4（ISO BMFF。QuickTime の `.mov` を含む）と WebM。映像の codec は H.264 / VP8 / VP9 / AV1、音声は AAC / Opus / Vorbis / MP3。HEVC は受け付けない（Firefox などが再生できない。要検証）。
  - 音声: MP3、Ogg（Vorbis / Opus）、M4A（AAC）、WebM（Opus）。
  - codec は container のヘッダ（`stsd` のサンプル記述、WebM の `CodecID`）から読み、許可リストにないものは拒否する。
- **上限**（インスタンス設定、既定値）: 100 MiB（ADR-0019 §8）、動画の長さ 5 分・音声 10 分、映像の面積 3840×2160・120 fps まで（Mastodon の `MAX_VIDEO_MATRIX_LIMIT` / `MAX_VIDEO_FRAME_RATE` と同じ）。
- **トランスコードしない**（ADR-0005 の方針を保つ）。サーバーは純 Go で container を解析し、
  - MP4 は `udta` / `meta` などのメタデータの box（iPhone の位置情報 `©xyz` を含む）を落とし、`moov` を先頭に置き直して（fast start）`stco` / `co64` の位置を直す。
  - WebM は `Tags` と `Attachments` の要素を落とす。MP3 は ID3v1 / ID3v2、Ogg は Vorbis comment / OpusTags の中身を落とす。
  - 長さ・寸法・codec はこの解析で得る。候補は [`github.com/abema/go-mp4`](https://pkg.go.dev/github.com/abema/go-mp4)（MIT）と [`github.com/at-wat/ebml-go`](https://pkg.go.dev/github.com/at-wat/ebml-go)（Apache-2.0）。どちらも読み書きに対応と記載（2026-10-20 確認）。MP3 / Ogg は自前で読む。
- **ポスター**: Web クライアントが upload のときに `<video>` と `<canvas>` で 1 フレームを取り出し、画像として一緒に送る（Mastodon の API の `thumbnail` と同じ考え方）。サーバーはそれを §1 の投稿画像として処理して静止版の S / M / L と blurhash を作る。
  音声は、利用者が選んだ画像か、ファイルに埋め込まれたカバー画像（ID3 の `APIC`、MP4 の `covr`）をポスターにする。ポスターがなければ、クライアントは「プレビューなし」と表示する（federation.md §9 といまと同じ）。
- 波形は作らない（AAC / Opus を純 Go で復号できないため）。
- Safari での再生は配信の挙動に任せる。`Range` のない配信（いまのメディアプロキシ）や Safari が扱わない WebM の codec で再生できない場合も、この ADR では対応を足さず、そのままにする。

V2 / V3 の外部の `ffmpeg` や WASM 版はこの ADR では入れない。動画対応の拡張（V2 などのトランスコードのアダプタ）は今後の予定とし、入れるときに別の ADR で、HEVC の受け付けと Safari での再生の範囲もあわせて決め直す。

### 7. リモートのメディア（R2）

- メディアプロキシの `variant` に `small` を足す。リモートの静止画・アニメーション画像の最初のフレームを、§1 の用途（記録したときの用途: 添付・アイコン・ヘッダー・絵文字）の S に縮めた静止版を返す。
  これでタイムラインのリモートのアイコンが小さくなり、`prefers-reduced-motion` のときにリモートのアニメーションのアイコン・絵文字を止められる。
- `small` はプロキシが取得のたびに作り、`Cache-Control` を付けて CDN に任せる（いまの `full` と同じ）。`remote_media_policy` が `avatars` / `all` のときは、`full` と同じ `remote_media_cache` に派生のキーで保存する。
- 動画・音声・`gifv` は派生させない。ポスターはリモートの `Document` の `icon` を別のリモートの画像として記録し、`small` で返す。
- M / L は作らない。`full` を返す。

### 見直す条件

- WebP の spike が通らない → S / M も JPEG / PNG（A1）。
- 純 Go の remux が主要な端末（iPhone・Android の標準カメラ）の動画で壊れる、または Mastodon が再生できない動画が多い → 予定している動画対応の拡張（V2 など）の ADR を前倒しで起こす。
- プロキシの縮小が 1 vCPU の構成で重すぎる → R1 に戻し、クライアントの `full` の縮小表示だけにする。

### ADR-0005 のうち置き換える部分

この ADR は [ADR-0005](0005-blob-storage-and-media.md) の次の部分を置き換える。BlobStore ポートとリモートメディアのキャッシュの方針（既定でキャッシュしない、TTL 付きのキャッシュ、プロキシ）はそのまま残る。

- 「MediaProcessor ポート」の責務「画像の EXIF 除去・リサイズ・サムネイル・blurhash 生成・形式検証」→ §1〜§3 の S / M / L とアニメーションの静止版・動く版、§6 の動画・音声の検証とメタデータの除去（remux）を加えた責務。
- 「リモートメディア方針」のうち、プロキシが中継だけをすること → §7 の `small` の派生。
- Consequences の「動画のトランスコードは v0 では行わない（サイズ・形式制限のみ）」→ §6（トランスコードはしないが、codec の検証とメタデータの除去・remux をする）。

## Consequences（影響）

- 良い影響
  - タイムラインのアイコン・画像の転送量が減る（アイコンは 400 px から 96 px へ、投稿画像は L から S / M へ）。不変のキーで CDN に長くキャッシュできる。
  - `prefers-reduced-motion` と「自動再生しない」設定を、ローカル・リモートのどちらのアニメーションにも当てられる。
  - 動画・音声を cgo も外部コマンドもなしで受け付けられ、位置情報を落とせる。#629 を進められる。
- 悪い影響・トレードオフ
  - 保存量は静止画で L の約 1.3 倍、アニメーションでは 6 つ分に増える。
  - トランスコードしないので、HEVC の動画（iPhone の既定の「高効率」）は受け付けない。iOS の Safari が upload のときに H.264 に変換しなければ、利用者に設定の変更を案内する必要がある。
  - Safari で再生できない動画（`Range` のないプロキシ経由、WebM の一部の codec）が残る。
  - 純 Go の WebP エンコーダは利用者が少なく、保守が止まる危険がある。出力だけに使い、外れても A1 に戻せるようにする。
  - container の remux を自前で持つので、MP4 / WebM のテストとファズが必要。
  - プロキシの `small` はリクエストのたびに画像を復号するので、CDN のキャッシュに当たらないと CPU を使う。
- 後続で必要になる作業（Issue 化する）
  - #654: S / M / L とアニメーションの静止版・動く版の保存（§1〜§5）。`media_variants` のスキーマ、`MediaVariants` と Account の API、Web の `srcset` / `<picture>`、data-model.md と federation.md の更新を含む。
  - spike: 純 Go の WebP エンコーダ（静止・アニメーション）とアニメーション WebP のデコード（ADR-0022 に従い spike ブランチ）。
  - #629: 動画・音声の添付（§6）。MP4 / WebM / MP3 / Ogg の解析・remux とファズ、ポスターの upload、`Range` の配信。
  - メディアプロキシの `small`（§7）。
  - 動画対応の拡張（V2 などのトランスコードのアダプタ、HEVC の受け付け、Safari での再生の範囲）の計画。予定だけを Issue にし、着手は別の ADR で決める。
  - 設定を変えたときの全メディアの処理し直しのコマンドと、古い処理結果の Blob の掃除。

## Pros and Cons of the Options（詳細比較・任意）

| 選択肢 | 良い点 | 悪い点 |
| --- | --- | --- |
| A1 JPEG / PNG / GIF | 標準ライブラリだけ。どこでも表示できる | 同じ画質で WebP より大きい（要検証）。縮小した GIF は大きい |
| A2 S / M を WebP | 表示用の派生が小さい。`CGO_ENABLED=0` のまま | 若い依存。spike が要る |
| A3 WASM の libwebp / libaom | 成熟した実装。AVIF も出せる | バイナリが大きくなり、1 枚の CPU が重い。1 vCPU の構成に向かない（要検証） |
| V1 remux のみ | 単一バイナリのまま。処理が軽い（復号しない） | HEVC など再生できない codec を断るしかない。ポスターはクライアント頼み |
| V2 外部の `ffmpeg` | Mastodon と同じ。どの入力も H.264 / AAC にできる。ポスターも作れる | 実行環境に `ffmpeg` が要る（Docker イメージが大きくなる、serverless で使いにくい）。トランスコードの CPU と時間。H.264 の特許の扱い（要検証） |
| V3 WASM の ffmpeg | 単一バイナリでトランスコードできる（GoToSocial の方式） | 埋め込むバイナリが約 11 MB × 2、wazero のコンパイラが要る、CPU が重い。ライセンスは GPL-3.0（AGPL と両立）で、版ごとに要検証 |
| R1 派生させない | 実装が要らない | リモートのアイコンを原寸で配る。リモートのアニメーションを止められない |
| R2 プロキシで S | 小さく、止められる。保存は増えない（キャッシュの設定に従う） | プロキシで復号する CPU |
| R3 取り込み時に保存 | 表示が速い | 既定でキャッシュしない方針（ADR-0005、NFR-COST-4）に反する |

各サーバーの方式（2026-10-20 確認）:

- Mastodon: 画像は `original`（面積 3840×2160）と `small`（面積 640×360）。動画は `ffmpeg` で H.264 / AAC の MP4 にし（H.264 / AAC の入力は copy）、ポスターは最初のフレーム。アニメーション GIF は MP4（`gifv`）にする。アイコンは 400×400、ヘッダーは 1500×500 で、GIF にだけ静止版 `static` を作る（[media_attachment.rb](https://github.com/mastodon/mastodon/blob/main/app/models/media_attachment.rb)、[avatar.rb](https://github.com/mastodon/mastodon/blob/main/app/models/concerns/account/avatar.rb)、[header.rb](https://github.com/mastodon/mastodon/blob/main/app/models/concerns/account/header.rb)）。
- Misskey: `webpublic`（2048×2048 に収めた WebP / PNG）とサムネイル（498×422 の WebP、アニメーションは 374×317 の動く WebP）。動画にはサムネイルだけを作り、`webpublic` は作らない（[DriveService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/core/DriveService.ts)）。
- GoToSocial: WASM 版の ffmpeg（`go-ffmpreg`、wazero）で調べ、サムネイルは既定で WebP（不透明な JPEG / GIF / PNG / WebP は純 Go で JPEG）（[thumbnail.go](https://codeberg.org/superseriousbusiness/gotosocial/src/branch/main/internal/media/thumbnail.go)、[go-ffmpreg](https://codeberg.org/gruf/go-ffmpreg)）。リモートのメディアは一時的にキャッシュする（[Media caching](https://docs.gotosocial.org/en/latest/admin/media_caching/)）。
- Pleroma / Akkoma: 要検証（リモートの添付の表示は Document の `url` を使うと想定）。

## Open Questions / 要検証

- Mastodon・Misskey・GoToSocial・Akkoma が、`Document` の `icon`（ポスター）と `duration` を読むか、こちらが送る WebM・MOV 由来の MP4 を再生できるか（federation-live で確かめる）。
- iOS の Safari が upload のときに HEVC の動画を H.264 に変換するか。動画対応の拡張の ADR で使う。
