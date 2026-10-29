# ADR-0121: KMS の KeyManager（gcpkms / awskms）: 導出鍵を DB に包んで置き、包みを解いた DEK を短くキャッシュする

- Status: Proposed
- Date: 2026-10-29
- Deciders: @rosylilly
- Issue: #1842

## Context（背景・課題）

[ADR-0019 §6](0019-security-baseline.md#6-秘密情報の保管とアクター秘密鍵の暗号化nfr-sec-7)（NFR-SEC-7）は、DB の秘密をエンベロープ暗号化し、
`KeyManager` のアダプタを `local`（設定の KEK。既定）と、任意の `gcpkms` / `awskms`（KEK を KMS に置く）にすると決め、KMS のアダプタを後続の Issue に回した。
設定の `key_manager` は今 `gcpkms` / `awskms` を「not in v0」として拒む。M6 の Google Cloud・AWS のターゲットでは、KEK を環境変数やシークレットに置かず、
KMS に置いて IAM で使わせたい。

`kernel.KeyManager`（`CurrentKeyID` / `WrapKey` / `UnwrapKey`）を KMS の Encrypt / Decrypt に写すこと自体は素直だが、次の点が ADR-0019 で決まっていない。

- **KEK から導出している鍵**: メディアプロキシの URL の署名鍵は、`kek` と `kek_previous` から HKDF-SHA256 で導出している（`internal/platform/setup_media.go`）。KMS では KEK を手元に持たない
- **呼び出しの回数**: `kernel.Sealer.Open` は値を開くたびに `UnwrapKey` を呼び、結果を持たない。連合の配送と署名付きの取得は、要求のたびにアクターの秘密鍵を開く
  （`account.Service.SigningKey`）。KMS では要求ごとに Decrypt を 1 回呼び、その遅延・可用性・料金が配送に乗る。webhook の配送・TOTP の検証も同じ
- **鍵 ID**: 封じた値のヘッダーは鍵 ID を 1 バイトの長さで持つ（1〜255 バイト）。KMS の鍵の名前と版、KMS 側のローテーションとの関係
- **移行**: `local` から KMS へ、KMS から別の KMS の鍵へ、`my kek status` / `my kek rewrap` で移す方法

料金（2026-10-29 確認）: AWS KMS は鍵 1 つ月 $1（ローテーションごとに月 $1 を足し、2 回目で上限）、対称鍵の要求 1 万回 $0.03、無料枠は全リージョンで月 2 万要求（[AWS KMS pricing](https://aws.amazon.com/kms/pricing/)）。
Cloud KMS はソフトウェアの鍵の版 1 つ月約 $0.06、暗号操作 1 万回 $0.03、無料枠は Autokey で作った鍵だけ（[Cloud KMS pricing](https://cloud.google.com/kms/pricing)。時間あたりの単価から換算）。

今決める理由: M6 のテンプレートが KEK をどこに置くかを決める。メディアプロキシの鍵の置き場所を変えるなら、既存の URL の署名が変わるので v0.1.0 より前に変えたい（[ADR-0087](0087-schema-changes-after-launch.md)）。

## Decision Drivers（判断基準）

1. **KEK を手元に置かない**: KMS を選んだ構成では、プロセスの設定とディスクに KEK も、KEK から導出した鍵も置かない（ADR-0019 §6 の目的）
2. **配送を KMS の遅延と障害から守る**: 配送・取得の要求ごとに KMS を呼ばない。KMS が短く落ちても連合が止まらない
3. **運営者に新しい秘密を持たせない**: KMS を選ぶと、別の秘密（署名鍵など）の発行と保管が増える、ということにしない
4. **費用**: 小さなサーバーで無料枠か月数十セントに収まる（NFR-COST）。鍵そのものの月額は避けられない
5. **移行ができる**: `local` で動いているサーバーを、秘密を失わずに KMS に移せる。逆も
6. **境界**: SDK はアダプタと `cmd/` に閉じる。`CGO_ENABLED=0` を保つ

## Considered Options（選択肢）

論点ごとに選択肢を挙げる。

### 1. メディアプロキシの署名鍵

- **1A. 乱数の鍵を 1 度だけ作り、`Sealer` で封じて DB に置く**。起動時に 1 回開く。`key_manager` によらず同じにする
- **1B. 別の設定 `media_proxy_key`（と `_previous`）を、KMS のときに必須にする**
- **1C. KMS の MAC の操作（Cloud KMS の `MacSign`、AWS KMS の `GenerateMac`）で URL ごとに署名する**

### 2. `UnwrapKey` の呼び出し

- **2A. 包みを解いた DEK を、プロセスのメモリに短く（既定 10 分）上限つきでキャッシュする**。`kernel.KeyManager` を包むデコレータにし、どのアダプタにも使える
- **2B. キャッシュしない**。要求ごとに KMS を呼ぶ
- **2C. 開いた秘密（アクターの秘密鍵など）をモジュールの側でキャッシュする**

### 3. 移行

- **3A. 現在の `KeyManager` と、包みを解くだけの前の `KeyManager` たちを、鍵 ID の接頭辞で振り分ける**。`my kek rewrap` が今の仕組みのまま移す
- **3B. 移行用の専用のコマンドで、両方の設定を読んで一度に包み直す**

## Decision（決定）

> Proposed の推奨案。Accepted にするのは人間の Decider のみ。

**推奨案は 1A・2A・3A。**

### 1. アダプタ

- `internal/adapter/keymanager/gcpkms`（`cloud.google.com/go/kms`）と `internal/adapter/keymanager/awskms`（aws-sdk-go-v2 の `kms`）。認証は ADC と SDK の既定の資格情報の連鎖。
  どちらの SDK も Apache-2.0（要検証: 依存の推移的なライセンス）。ビルドタグ `no_gcpkms` / `no_awskms`
- `WrapKey` は KMS の Encrypt で 32 バイトの DEK を包み、`UnwrapKey` は Decrypt で開く。KEK は KMS の外に出ない
- **AAD**: Cloud KMS は `additionalAuthenticatedData` にそのまま渡す。AWS KMS は暗号化コンテキスト（文字列の map）に `{"my-aad": <aad の base64url>}` として渡す。
  暗号化コンテキストは CloudTrail のログに平文で残るが、AAD は行の ID と列の名前（`kernel.AAD`）で秘密を含まない
- **鍵 ID**: `gcpkms:<CryptoKey のリソース名>`、`awskms:<鍵の ARN>`。
  - Cloud KMS は版を含めない。Decrypt は暗号文から版を選ぶので、KMS 側の自動のローテーションで鍵 ID は変わらず、包み直しも要らない（要検証: Decrypt が鍵の名前だけで古い版の暗号文を開けること）
  - AWS KMS は起動時に `DescribeKey` で別名（alias）を ARN に直してから使う。別名は付け替えられるので鍵 ID にしない
  - 255 バイトを超える名前は起動時の設定の検査で拒む
- **エラー**: 暗号文や AAD が合わない（Cloud KMS の `InvalidArgument`、AWS KMS の `InvalidCiphertextException`）は `ErrDecrypt`。
  鍵 ID が今の鍵でも前の鍵でもないときは `ErrUnknownKey`。権限の不足・鍵の無効化は設定の誤りとしてそのまま返し、上限・一時的な失敗は再試行できるエラーにする

### 2. メディアプロキシの署名鍵を DB に封じて置く（1A）

- 32 バイトの乱数の鍵を、最初の起動で作って `Sealer` で封じ、インスタンスの秘密の表に置く（resource。行は名前で一意。ADR-0020）。起動時に 1 回開き、メモリに持つ
- `key_manager = local` も同じにする。KEK の HKDF からの導出をやめ、鍵の出どころを 1 つにする。この変更で、変更の前に署名した URL は一度だけ無効になる（v0.1.0 の前なので許す。プロキシの URL は API が出し直す）
- 封じた鍵は `my kek rewrap` の対象の列に足す。KEK を替えても署名鍵は変わらない
- 署名鍵そのもののローテーションは、この ADR の対象外にする（今の HKDF の導出でも KEK のローテーションに付いて回っていただけで、独立の要件はない）

### 3. 包みを解いた DEK のキャッシュ（2A）

- `kernel` に `KeyManager` を包むデコレータを置き、`UnwrapKey` の結果を `(鍵 ID, 包んだ DEK, AAD)` のハッシュを鍵にキャッシュする。既定は 10 分・1,024 件で、設定で変えられる（0 で無効）
- `WrapKey` はキャッシュしない（新しい秘密を封じるときだけで、回数が少ない）
- `key_manager = local` では使わない（KEK がメモリにあり、キャッシュの意味がない）
- 安全面: キャッシュが持つのは DEK で、プロセスのメモリを読める攻撃者には、`local` では KEK そのものが、KMS では有効期間内の DEK が見える。KMS の構成でもこれは `local` より悪くならない。
  キャッシュの値はログ・エラーに出さない（`slog.LogValuer` で `[REDACTED]`）
- 呼び出しの見積り: 配送・取得で使う秘密（ローカルのアクターの秘密鍵、インスタンスのアクター）1 つにつき、プロセスあたり 10 分に 1 回。
  10 個のアクターが常に配送するサーバーで、プロセス 1 つなら月約 4.3 万回（30 日 × 144 回 × 10）で、AWS KMS の無料枠を超えた分は月 $0.07 ほど。キャッシュがないと、配送・取得の数と同じだけ呼ぶ

### 4. 移行（3A）

- `kernel` に、包むのは現在の `KeyManager`、開くのは鍵 ID の接頭辞（`local:` / `gcpkms:` / `awskms:`）で選んだ `KeyManager`、という振り分けを置く
- 設定: `key_manager` が現在。`kek_previous`（と、`key_manager` が `local` でないときの `kek`）は開くだけの `local` の KEK。
  新しい設定 `kms_key`（現在の KMS の鍵）と `kms_key_previous`（開くだけの同じ事業者の KMS の鍵のリスト）
- `local` から KMS へ: `key_manager = gcpkms`・`kms_key` を設定し、今の `kek` を `kek_previous` に移して起動し、`my kek rewrap` を流す。`my kek status` が 0 件になったら `kek_previous` を外す。逆も同じ手順
- 違う事業者の KMS への移行（Cloud KMS から AWS KMS）は、`local` を経由する 2 回の移行で行う。直接の移行は作らない（一度に 2 つの SDK の資格情報を持たせない）

### この推奨を覆す条件

- インスタンスの秘密の表を置くことが、ほかのモジュールの表の持ち方と合わないと分かった場合（1B に倒す）
- 運用上、DEK をメモリに置く時間を 0 にする要件が出た場合（2B。配送の遅延と KMS の費用を受け入れる）

## Consequences（影響）

- 良い: KMS の構成では、プロセスの設定とディスクに KEK も導出した鍵もない。KEK の漏洩の経路が IAM の権限に限られ、KMS の監査ログに使われ方が残る
- 良い: 配送の要求が KMS を待たない。KMS の短い障害は、キャッシュの有効期間の内なら配送を止めない
- 良い: 運営者は KMS の鍵を 1 つ用意するだけで、新しい秘密を持たない
- 悪い: KMS の鍵に月額がかかる（AWS は月 $1〜$3、Cloud KMS は版ごとに月約 $0.06）。KMS は任意で、`local` が既定のまま
- 悪い: キャッシュの有効期間の間、DEK がメモリに残る。無効にもできる
- 悪い: キャッシュの有効期間を超える KMS の障害では、秘密を開けず、配送・ログインの 2 段階認証・webhook が失敗する（ジョブは再試行される）。起動時のメディアプロキシの鍵も開けず、起動が失敗する
- 悪い: メディアプロキシの鍵の置き場所を変えると、今のサーバーでは変更の前の URL が一度だけ無効になる
- 後続の作業（#1843・#1844 の Scope をこの ADR に合わせて書き直す）:
  - `kernel` の振り分けとキャッシュのデコレータ、インスタンスの秘密の表とメディアプロキシの鍵の移し替え、`rewrap` の対象の列の追加（共通の部分。どちらかの issue に含めるか、別の issue にする）
  - `keymanager/gcpkms`（#1843）、`keymanager/awskms`（#1844）、`KeyManager` のコントラクトテスト（KMS の API はテスト用のサーバーで差し替える）
  - `docs/operations/key-management.md` の「`key_manager` is `local` only」と、HKDF の導出の記述、KMS の権限（Encrypt・Decrypt・DescribeKey だけ）と移行の手順
  - この ADR が Accepted になったら、[ADR-0019](0019-security-baseline.md) の Status の行に「§6 の KMS のアダプタは ADR-0121 が決める」と添える（本文は書き換えない。ADR-0086 の Status の書き方と同じ）

## Pros and Cons of the Options（詳細比較・任意）

### 1. メディアプロキシの署名鍵

- **1A（推奨）**: 判断基準 1・3 を満たす。起動時に KMS を 1 回呼ぶ。悪い: 新しい表と、変更の前の URL の一度の無効
- **1B**: 表が要らず、署名鍵を独立にローテーションできる。悪い: KMS を選んだのに環境変数の秘密が残り、判断基準 1・3 に反する
- **1C**: 鍵が KMS の外に出ない。悪い: プロキシの URL を出すたび・検証するたびに KMS を呼び、判断基準 2・4 に強く反する

### 2. `UnwrapKey` の呼び出し

- **2A（推奨）**: 1 か所で全モジュールに効く。悪い: DEK がメモリに残る
- **2B**: メモリに残らない。悪い: 配送 1 件ごとに KMS の遅延（要検証: 1 回の Decrypt の遅延）と料金が乗り、KMS の障害で連合がすぐ止まる
- **2C**: 開いた署名鍵をそのまま再利用でき、`x509` の解析も省ける。悪い: 秘密の種類ごとにモジュールがキャッシュを持ち、有効期間と消去の規則がばらつく。秘密鍵そのものがメモリに長く残る

### 3. 移行

- **3A（推奨）**: 今の `kek_previous` と `my kek rewrap` の考え方をそのまま広げ、無停止で少しずつ移せる。悪い: 移行の間、2 つの鍵の出どころの設定が要る
- **3B**: 設定の意味が単純。悪い: 移行が一度きりの操作になり、途中で止まったときの再開と、移行中に新しく封じた値の扱いが要る

## Open Questions / 要検証

- Cloud KMS の Decrypt が、鍵の名前だけで古い版で包んだ暗号文を開けること（§1 の鍵 ID）
- Cloud KMS の `additionalAuthenticatedData` と AWS KMS の暗号化コンテキストの大きさの上限が、`kernel.AAD` の値に足りること
- Cloud Run・Lambda からの 1 回の Decrypt の遅延と、AWS KMS の要求のクォータ（アカウントで共有）
- `cloud.google.com/go/kms` と aws-sdk-go-v2 の `kms` の推移的な依存のライセンスと、バイナリの大きさの増え方
