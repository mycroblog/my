# ADR-0028: Go が HTML の外枠を返し、リンクプレビュー（OGP / X Card）と PWA を両立する

- Status: Accepted（外枠の `GET /@{acct}/{id}` を ADR-0053（投稿の表示用 URL） が、更新の通知の部分を ADR-0141 が置き換える）
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #88

## Context

Web クライアントは React の SPA で、インストールできる PWA にする（ADR-0008）。
同時に、公開プロフィール（`/@alice`）と公開投稿（`/@alice/{id}`）のリンクを X・Mastodon・Discord・Slack・LINE などに貼ったとき、プレビュー（OGP、X Card）が出る必要がある（オーナーの要件）。
プレビューを取得するボットは JavaScript を実行しないので、SPA だけでは meta タグを返せない。
サーバーは Go の単一バイナリで、低コスト（NFR-COST-3）を前提とする。

## Considered Options

1. **Go がルートごとに HTML の外枠を `html/template` で返し、meta タグを埋め込んでから SPA を起動する**
2. JS の実行環境でのサーバーサイドレンダリング（Bun / Node を併走させる）
3. 事前レンダリング（投稿は実行時に増えるので合わない）
4. ボットだけに別の HTML を返す動的レンダリング（Google が非推奨）

## Decision

**1 を採用する。**

### HTML の外枠

- `my serve` がすべての HTML を返す。
  - `GET /@{acct}` と `GET /@{acct}/{id}` は meta タグ付きの外枠を返す。
  - それ以外の SPA のルートは、汎用の外枠を返す。
- ActivityPub のコンテントネゴシエーション（ADR-0018）を先に判定する。
  - `application/activity+json` / `application/ld+json` なら AP の JSON を返し、それ以外なら HTML を返す。
  - 常に `Vary: Accept` を付け、HTML には `<link rel="alternate" type="application/activity+json">` を入れる。
- **HTML のリクエストは常に未ログインとして扱う。** API は Bearer トークンだけで認証し、HTML の外枠は個人ごとに変えない。
  - 未ログインでも見られる投稿（`public` と `unlisted`）にだけ、プレビュー用の meta タグを付ける。
    - `unlisted` には `noindex` を付ける。
  - `local` / `private` / `direct` / 削除済み / 存在しない投稿は、**どれも同じ汎用の外枠を 404 と `X-Robots-Tag: noindex` で返す**（存在を漏らさない。ADR-0017, ADR-0018）。
    - ログイン中の閲覧者には、SPA が起動後にトークン付きで投稿を取得して表示する。
- テンプレートは `html/template` の文脈に応じたエスケープに任せる。利用者のデータを `template.HTML` に変換しない。
  - `og:description` は、投稿の HTML からタグを除いた平文を約 200 文字に切り詰めて使う。
- 外枠の HTML は `Cache-Control: public, max-age=60, stale-while-revalidate=300` と ETag で返す。
  - ETag はテンプレートの版・アセットの版・投稿の版・公開範囲・ロケールから作る。
- CSP の nonce は使わない（キャッシュできなくなるため）。
  - インスタンスごとの設定（API のオリジン、インスタンス名など）は、`<script type="application/json">` のデータブロックで渡す。
  - ビルド成果物はどのインスタンスでも同じにし、`import.meta.env` にインスタンスごとの値を入れない。

### リンクプレビューの meta タグ

- **OGP**
  - `og:title`, `og:type`（プロフィールは `profile`、投稿は `article`）, `og:url`（正規 URL）, `og:image`（と `og:image:alt` / `width` / `height`）, `og:description`, `og:site_name`, `og:locale` を出す。
  - 投稿には `article:published_time` と `article:author` も出す。
- **X Card**: `twitter:card` を出す。画像の比率が合えば `summary_large_image`、なければ `summary` にする。そのほかの項目は X が OGP から読む。
- **Fediverse**: `fediverse:creator`、`<link rel="canonical">` を出す。
- `og:image` は次の順で選ぶ。カード画像の生成は後で検討する。
  1. 投稿の最初の添付画像（代替テキスト付き）
  2. プロフィールのヘッダー画像、またはアバター
  3. インスタンスの既定画像
- `<html lang>` と `og:locale` は投稿の言語（またはアカウントの既定）から決める。`Accept-Language` では変えない（キャッシュを分けないため）。

### PWA

- マニフェストはインスタンスの設定から Go が実行時に生成する（名前・テーマ色・言語）。
- Service Worker は `vite-plugin-pwa` の `injectManifest` で、自前の `sw.ts` を使う（Web Push に備える）。
  - **ハッシュ付きのアセット**: 事前にキャッシュする。
  - **ページ遷移**: ネットワーク優先にする（3 秒でタイムアウトし、オフライン用の外枠に切り替える）。サーバーが返す 404 や `noindex` を正とするため。
  - **`/api/*`**: キャッシュしない（NetworkOnly）。ログアウトしたら、すべてのキャッシュを消す。
  - **公開メディアとアバター**: キャッシュ優先にし、件数と期限に上限を設ける。
  - Service Worker で扱わないパス: `/api/`, `/oauth/`, `/.well-known/`, `/nodeinfo`, `/ap/`, `/media/`, `/manifest.webmanifest`
- 更新は「新しい版があります」と通知し、利用者の操作で反映する（下書き中の自動リロードを避ける）。
  - デプロイで古いチャンクが消えたとき（`vite:preloadError`）は、ページを読み直す。

### 配信

- Vite は `build.manifest: true` と相対の `base` でビルドする。Go はマニフェストから `<script>` / `<link>` を組み立てる。
  - 埋め込みは `//go:embed all:dist`（`.vite/` を含めるため）。CDN から配信するときは `/assets/*` だけを置く。
- ハッシュ付きのアセットは `immutable` で 1 年キャッシュする。外枠の HTML・`sw.js`・マニフェストは長期キャッシュしない。

## Consequences

- プレビューと PWA を、JS の実行環境なしで両立できる。
- CDN を前に置く場合は、`Accept` の種類をキャッシュキーに含める必要がある（AP の JSON と HTML を取り違えないため）。デプロイ文書に書く。
- プレビューの表示は X・Mastodon・Discord・Slack・LINE の実機で確認する（手動、リリースごと）。

## Open Questions / 要検証

- [ ] 投稿のカード画像を生成するか（CJK フォントでバイナリが 10〜20 MB 増える。CPU コスト）
- [ ] oEmbed を提供するか
