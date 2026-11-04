# ADR-0005: Blob ストレージとメディア処理

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #7, #644

## Decision（決定）

### 1. BlobStore ポート

- 操作: `put` / `get`（ストリーム）/ `delete` / `head` / `publicUrl` / `signedUrl`（任意）
- アダプタ: `blob/fs`（ローカル FS）、`blob/s3`（S3 互換 API: AWS S3 / Cloudflare R2 / GCS の XML 互換 API / MinIO / その他）。必要なら `blob/gcs`（ネイティブ API）
- 配信は可能な限り CDN / オブジェクトストレージの公開 URL から直接行い、アプリを経由させない。

### 2. MediaProcessor ポート

- 責務: 画像の EXIF 除去・リサイズ・blurhash 生成・形式検証、§4〜§6 の S / M / L とアニメーションの静止版・動く版、§9 の動画・音声の検証とメタデータの除去（remux）
- アダプタ: `media/native`（pure Go: `golang.org/x/image` 等。既定）、`media/vips`（libvips、cgo。ビルドタグで任意）、
  `media/none`（無加工。ただし EXIF 除去だけは軽量実装で必須）
- 自前クライアント（Web / モバイル）は **アップロード前にクライアント側でリサイズ・EXIF 除去** する。サーバー側処理は保険。

### 3. リモートメディア方針（FR-409, NFR-COST-4）

- 既定: **キャッシュしない**。クライアントには自インスタンスの **メディアプロキシ URL** を返し、プロキシは CDN キャッシュヘッダを付けて中継する
  （利用者の IP をリモートに漏らさない、かつストレージを消費しない）。静止画の小さな派生だけはプロキシが作る（§10）。
- オプション: TTL 付きでキャッシュ（アバター・ヘッダーのみ / 全メディア）、上限容量を設定可能。
- ドメインブロックの「メディア拒否」はプロキシ層で適用。

### 4. 画像の派生の段と大きさ

派生は用途ごとに次の箱に作る。アイコンは**正方形に切り抜いて**から箱に縮め、切り抜く位置は焦点（`media_focus`、なければ中央）に合わせる。それ以外は**縦横比を保って収め**（切り抜かない。表示側が `object-fit: cover` と焦点で切る）。箱より小さい元画像は拡大しない。

| 用途 | S | M | L |
| --- | --- | --- | --- |
| 投稿の画像 | 面積 640×360 | 面積 1920×1080 | 面積 3840×2160 |
| アイコン | 96×96 | 192×192 | 400×400 |
| ヘッダー | 750×250 | 1500×500 | 3000×1000 |
| カスタム絵文字 | 64×64 | 128×128 | 256×256 |

- 投稿画像の S と L は Mastodon の `small` / `original` と同じ面積（[media_attachment.rb](https://github.com/mastodon/mastodon/blob/main/app/models/media_attachment.rb)）。M は 1080p の画面で全面に出す大きさ。
- アイコン L とヘッダー M は Mastodon の 400×400 / 1500×500 に合わせる。アイコンを正方形に切り抜くのも Mastodon と同じ（`400x400#`）（[avatar.rb](https://github.com/mastodon/mastodon/blob/main/app/models/concerns/account/avatar.rb)、[header.rb](https://github.com/mastodon/mastodon/blob/main/app/models/concerns/account/header.rb)。2026-10-20 確認）。S / M はタイムラインの 48 px と 96 px を 2 倍の解像度で埋める大きさ。
- カスタム絵文字の受け付けの上限と登録の手順は ADR-0051 が決め、この表は派生の箱だけを決める。
- 用途ごとの箱は `media.Sizes` に持たせ、upload の API が用途（`attachment` / `avatar` / `header` / `emoji`）を受け取る。用途を変えて使うときは、その用途で処理し直す（新しい処理結果）。
- 原本は保存しない（位置情報を残さないため）。L が保存する最大のもの。

### 5. 形式と画質

- **L**: JPEG（不透明、品質 85）/ PNG（透過）。連合先がどこでも表示できる形式にする。
- **S / M**: WebP（lossy、品質 80、透過は alpha 付き）。純 Go のエンコーダ [`github.com/deepteams/webp`](https://pkg.go.dev/github.com/deepteams/webp)（MIT、cgo なし）を使う。
  画質・速度・メモリ・ファズ（壊れた出力を作らないこと）、アニメーション WebP の読み書き、主要ブラウザでの表示を [#654 の spike](https://github.com/mycroblog/my/issues/654) で確かめて採用した。採用する版は、その時点でリリースから 3 日を過ぎたもの。
  WebP は主要ブラウザすべてで表示できる（Safari 14 以降。要検証）。
- AVIF は出力しない。純 Go のエンコーダがなく、WASM は 1 枚あたりの CPU が重い（要検証）。入力としての AVIF / HEIC も `media/native` は受け付けない（Mastodon も安全のため止めている。[media_attachment.rb](https://github.com/mastodon/mastodon/blob/main/app/models/media_attachment.rb)）。ADR-0019 §8 の許可リストは上限で、アダプタはその一部だけを受け付けてよい。
- blurhash は S から作る。

### 6. アニメーション画像

- 入力: GIF とアニメーション WebP。APNG は既定の画像（最初のフレーム）だけを読む静止画として扱う（標準の `image/png` の挙動）。
- フレーム数と全フレームの画素数の上限は `media.Limits`。
- アニメーションの入力は、**静止版（最初のフレーム）の S / M / L** と **動く版の S / M / L** の 6 つを作る。
  - 静止版は §5 の静止画と同じ形式。
  - 動く版の L は GIF（連合先が表示できる）、S / M はアニメーション WebP。縮小した GIF はフレームごとに元のパレットへ減色し直す。
- クライアントは `<picture>` の `<source media="(prefers-reduced-motion: no-preference)">` に動く版を、`<img>` に静止版を置く。利用者の設定「アニメーションを自動再生しない」が有効なら `<source>` を出さず、ホバー・フォーカス・タップで動く版に替える。
- 投稿の `gifv`（リモートの Mastodon が GIF を MP4 にしたもの）は動画として扱い、自動再生は同じ条件で止める。

### 7. 作る時機とデータモデル

- 派生は upload のあとのジョブ `media.process` で、1 回の処理で全部作る（ADR-0004。ジョブは冪等）。upload の応答は `processing` を返す。
- `media_variants`（イベント、`media_processing_results` の子）は `size`（`small` / `medium` / `large`）と `motion`（`still` / `animated`）の 2 列を持ち、主キーを `(result_id, size, motion)` にする（ADR-0020）。
  動画・音声のポスターも `motion = still` の派生として同じ表に入れる。
- Blob のキーは `media/{media_id}/{result_id}/{size}.{ext}` と `media/{media_id}/{result_id}/{size}-animated.{ext}`。処理し直すとキーが変わるので、配信は `Cache-Control: public, max-age=31536000, immutable` にできる。古い処理結果の Blob は掃除のジョブが消す（FR-803）。
- 大きさや形式の設定を変えたときの作り直しは、管理者のコマンドで全ファイルの `media.process` を入れ直す（後続 Issue）。

### 8. クライアント API と ActivityPub

- クライアント API の `MediaVariants` は `small` / `medium` / `large`（静止版、必須）と、アニメーションのときだけの `animated`（`small` / `medium` / `large`）にする。各派生は `url` / `content_type` / `width` / `height` / `size` を持ち、クライアントは `srcset` の幅指定で選ぶ。
  アカウントの画像も同じ形のオブジェクト（`avatar` / `header`）にする。
- ActivityPub の `attachment` の `Document` は、`url` に L（アニメーションなら動く版の L の GIF）、`mediaType`・`width`・`height`・`blurhash`・`focalPoint`・`name` を載せる。
  動画・音声はポスターの L を `icon`（`Image`）に、長さを `duration`（ISO 8601）に載せる。Mastodon も同じ形で送っている（[note_serializer.rb](https://github.com/mastodon/mastodon/blob/main/app/serializers/activitypub/note_serializer.rb) の `icon` と `duration`。2026-10-20 確認）。
  `url` を複数の `Link` の配列にはしない（受け側の対応が揃っていない。要検証）。
- Actor の `icon` はアイコンの L、`image` はヘッダーの L（アニメーションなら動く版）。静止版は受け側が作る（Mastodon は GIF から `static` を作る）。

### 9. 動画・音声

- **受け付ける形式**（中身で判定、ADR-0019 §8）:
  - 動画: MP4（ISO BMFF。QuickTime の `.mov` を含む）と WebM。映像の codec は H.264 / VP8 / VP9 / AV1、音声は AAC / Opus / Vorbis / MP3。HEVC は受け付けない（Firefox などが再生できない。要検証）。
  - 音声: MP3、Ogg（Vorbis / Opus）、M4A（AAC）、WebM（Opus）。
  - codec は container のヘッダ（`stsd` のサンプル記述、WebM の `CodecID`）から読み、許可リストにないものは拒否する。
- **上限**（インスタンス設定、既定値）: 100 MiB（ADR-0019 §8）、動画の長さ 5 分・音声 10 分、映像の面積 3840×2160・120 fps まで（Mastodon の `MAX_VIDEO_MATRIX_LIMIT` / `MAX_VIDEO_FRAME_RATE` と同じ）。
- **トランスコードしない**。サーバーは純 Go で container を解析し、
  - MP4 は `udta` / `meta` などのメタデータの box（iPhone の位置情報 `©xyz` を含む）を落とし、`moov` を先頭に置き直して（fast start）`stco` / `co64` の位置を直す。
  - WebM は `Tags` と `Attachments` の要素を落とす。MP3 は ID3v1 / ID3v2、Ogg は Vorbis comment / OpusTags の中身を落とす。
  - 長さ・寸法・codec はこの解析で得る。候補は [`github.com/abema/go-mp4`](https://pkg.go.dev/github.com/abema/go-mp4)（MIT）と [`github.com/at-wat/ebml-go`](https://pkg.go.dev/github.com/at-wat/ebml-go)（Apache-2.0）。どちらも読み書きに対応と記載（2026-10-20 確認）。MP3 / Ogg は自前で読む。
- **ポスター**: Web クライアントが upload のときに `<video>` と `<canvas>` で 1 フレームを取り出し、画像として一緒に送る（Mastodon の API の `thumbnail` と同じ考え方）。サーバーはそれを §4 の投稿画像として処理して静止版の S / M / L と blurhash を作る。
  音声は、利用者が選んだ画像か、ファイルに埋め込まれたカバー画像（ID3 の `APIC`、MP4 の `covr`）をポスターにする。ポスターがなければ、クライアントは「プレビューなし」と表示する（federation.md §9 と同じ）。
- 波形は作らない（AAC / Opus を純 Go で復号できないため）。
- Safari での再生は配信の挙動に任せる。`Range` のない配信（メディアプロキシ）や Safari が扱わない WebM の codec で再生できない場合も、対応を足さず、そのままにする。

外部の `ffmpeg` や WASM 版は入れない。動画対応の拡張（トランスコードのアダプタ）は今後の予定とし、入れるときに別の ADR で、HEVC の受け付けと Safari での再生の範囲もあわせて決め直す。

### 10. リモートのメディアの派生

- メディアプロキシの `variant` に `small` を持つ。リモートの静止画・アニメーション画像の最初のフレームを、§4 の用途（記録したときの用途: 添付・アイコン・ヘッダー・絵文字）の S に縮めた静止版を返す。
  これでタイムラインのリモートのアイコンが小さくなり、`prefers-reduced-motion` のときにリモートのアニメーションのアイコン・絵文字を止められる。
- `small` はプロキシが取得のたびに作り、`Cache-Control` を付けて CDN に任せる（`full` と同じ）。`remote_media_policy` が `avatars` / `all` のときは、`full` と同じ `remote_media_cache` に派生のキーで保存する。
- 動画・音声・`gifv` は派生させない。ポスターはリモートの `Document` の `icon` を別のリモートの画像として記録し、`small` で返す。
- M / L は作らない。`full` を返す。

### 11. 見直す条件

- 純 Go の remux が主要な端末（iPhone・Android の標準カメラ）の動画で壊れる、または Mastodon が再生できない動画が多い → 予定している動画対応の拡張の ADR を前倒しで起こす。
- プロキシの縮小が 1 vCPU の構成で重すぎる → 派生させずに `full` を中継する形に戻し、クライアントの `full` の縮小表示だけにする。

## 経緯

Fediverse サーバーの運営費で最も膨らみやすいのがリモートメディアのキャッシュ（ストレージと転送量）で、画像処理の libvips などは cgo を要求して ADR-0002 の静的ビルドと衝突する。そこで BlobStore と MediaProcessor をポートにし、純 Go の既定のアダプタと、キャッシュしないプロキシを決めた。

はじめは `full` と `thumbnail` の 2 段で、アイコンにも 3840×2160 までの画像を配ることがあり、リモートのメディアは `full` を中継するだけだった。決定者の指示（#644）で、S / M / L の 3 段とアニメーション GIF の静止版・動く版の保存、動画・音声の方針を決めた。基準は `CGO_ENABLED=0` の単一バイナリで 1 vCPU / 1 GB で動くこと（NFR-COST-3）、メタデータを公開しないこと（NFR-SEC-6）、連合先で表示・再生できること、配る量を減らすこと、動くものを止められること（WCAG 2.2 SC 2.2.2）である。

S / M の WebP は条件付きで決め、#654 の spike が通ったので確定した。

関連する要件: FR-105、FR-111、FR-409、NFR-COST-4、FR-803、NFR-SEC-6。

## 採らなかった案

- 全段を JPEG / PNG / GIF: 標準ライブラリだけで済み、どこでも表示できるが、同じ画質で WebP より大きい（#654 で約 2 倍）。純 Go のエンコーダが使えなくなったときの戻り先として残る。
- WebP と AVIF を WASM 版の libwebp / libaom（wazero）で作る: 成熟した実装で AVIF も出せるが、バイナリが大きくなり 1 枚の CPU が重く、1 vCPU の構成に向かない（要検証）。
- 外部の `ffmpeg` でトランスコード（Mastodon の方式）: どの入力も H.264 / AAC にでき、ポスターも作れるが、実行環境に `ffmpeg` が要り、CPU と時間がかかる。H.264 の特許の扱いも要検証。
- WASM 版の ffmpeg（`go-ffmpreg`、GoToSocial の方式）: 単一バイナリでトランスコードできるが、埋め込むバイナリが約 11 MB × 2 で wazero のコンパイラが要り、CPU が重い。ライセンスは GPL-3.0（AGPL と両立）で版ごとに要検証。
- リモートのメディアを派生させない: 実装が要らないが、リモートのアイコンを原寸で配り、リモートのアニメーションを止められない。
- 取り込み時に取得して S / M / L を保存する: 表示は速いが、既定でキャッシュしない方針（§3）に反する。

## Consequences（影響）

- 最小構成ではストレージ費用はローカルメディアの分だけになる。
- メディアプロキシは転送量を消費する（CDN キャッシュヒット率に依存）。Cloudflare R2 + CDN を Blob 先に選ぶと、転送量課金を抑えられる（他ターゲットと併用可）。
- タイムラインのアイコン・画像の転送量が減る。不変のキーで CDN に長くキャッシュできる。
- `prefers-reduced-motion` と「自動再生しない」設定を、ローカル・リモートのどちらのアニメーションにも当てられる。
- 動画・音声を cgo も外部コマンドもなしで受け付けられ、位置情報を落とせる。
- 保存量は静止画で L の約 1.3 倍、アニメーションでは 6 つ分に増える。
- トランスコードしないので、HEVC の動画（iPhone の既定の「高効率」）は受け付けない。iOS の Safari が upload のときに H.264 に変換しなければ、利用者に設定の変更を案内する必要がある。
- Safari で再生できない動画（`Range` のないプロキシ経由、WebM の一部の codec）が残る。
- 純 Go の WebP エンコーダは利用者が少なく、保守が止まる危険がある。出力だけに使い、外れても JPEG / PNG に戻せるようにする。
- container の remux を自前で持つので、MP4 / WebM のテストとファズが必要。
- プロキシの `small` はリクエストのたびに画像を復号するので、CDN のキャッシュに当たらないと CPU を使う。
- 後続（Issue 化する）: 動画対応の拡張の計画（予定だけを Issue にし、着手は別の ADR で決める）。設定を変えたときの全メディアの処理し直しのコマンドと、古い処理結果の Blob の掃除（#2181。`v0.1.0` の後）。

## Open Questions / 要検証

- GCS の S3 互換 API での署名付き URL 互換性（2026-10-16 確認: GCS は XML API で HMAC キーによる `AWS4-HMAC-SHA256` 署名をサポートし、署名付き URL は最大 7 日。aws-sdk-go-v2 の presigner・マルチパート・チェックサムヘッダとの実機互換は未確認）
- R2 の署名付き URL は S3 API ドメインでのみ有効で、カスタムドメイン（公開バケット）では使えない（2026-10-16 確認）。`publicUrl` と `signedUrl` で異なるホストを返す設計で問題ないか確認する（出典は [deployment-targets.md](../architecture/deployment-targets.md)）
- Mastodon・Misskey・GoToSocial・Akkoma が、`Document` の `icon`（ポスター）と `duration` を読むか、こちらが送る WebM・MOV 由来の MP4 を再生できるか（federation-live で確かめる）。
- iOS の Safari が upload のときに HEVC の動画を H.264 に変換するか。動画対応の拡張の ADR で使う。
