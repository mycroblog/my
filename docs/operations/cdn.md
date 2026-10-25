# メディア配信に CDN を置く

運営者向けの手順です。設定項目は [設定リファレンス](../reference/configuration.md#media_origin)、
方針は [ADR-0005](../adr/0005-blob-storage-and-media.md) と [NFR-COST-4](../requirements/02-non-functional.md) を参照してください。

メディアの配信（`/media/` と `/media/proxy/`）は、サーバーの前に置いた CDN（Cloudflare、Google Cloud CDN、CloudFront など）に任せられる。
CDN のキャッシュに当たった分は、サーバーの CPU と転送量を使わない。
ここで扱うのはサーバーを CDN のオリジンにする形（オリジンプル）で、`blob/s3` アダプタの公開 URL は含まない。

## 設定

CDN のホスト名（たとえば `media.example.org`）を `media_origin`（`MY_MEDIA_ORIGIN`）に設定する。

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- 値はオリジン（`scheme://host[:port]`）だけで、パス・クエリ・末尾のスラッシュは付けない。`https` が必須で、`http` は `localhost` などのループバックだけ使える。不正な値だと起動に失敗する。
- 未設定なら、メディアはサーバー自身のオリジン（`https://{host}`）から配る。
- 次の URL がこのオリジンになる。ローカルのメディアは `{media_origin}/media/...`、リモートのメディアのプロキシは `{media_origin}/media/proxy/...`。
  クライアント API、ActivityPub（`attachment`、`icon`、`image`、カスタム絵文字の `icon`）、リンクプレビュー（OGP）が出す URL のすべてが対象。
- Web クライアントの Content-Security-Policy は、`img-src` と `media-src` にこのオリジンを足す。
- ActivityPub のオブジェクトの `id` などの連合 URI は、`media_origin` に関係なく `https://{host}/` の下に残る（[ADR-0018](../adr/0018-federated-uris.md)）。メディアの URL ではなく、ファイルの置き場所だけが変わる。
- URL は読み出すたびに組み立てる。DB には保存していないので、`media_origin` を変えると、すでにあるメディアの URL も変わる。
  連合先が保存した古い URL は、オリジンの `/media/` が配信を続ける限り読める。

## CDN の向き先

- オリジン（バックエンド）は、サーバーの公開ホスト名（`https://{host}`）にする。CDN は `/media/` 以下だけをサーバーに取りに来ればよい。
- サーバーは `/media/` と `/media/proxy/` をホスト名で制限しない。CDN の `Host` がどれでも配信する。
- `/media/` は認証も [Authorized Fetch](../requirements/01-functional.md)（FR-408）も掛からない。署名のない CDN からのリクエストにもそのまま答える。
- リモートのメディア（`/media/proxy/{id}/{variant}?sig=...`）は、`sig` が URL の一部である。**キャッシュキーにクエリ文字列を含める**。含めないと、署名の違う URL が同じエントリを共有する。
- キャッシュの期間は、オリジンの `Cache-Control` に従わせる（CDN 側で上書きしない）。

| パス | `Cache-Control` |
| --- | --- |
| `/media/{...}`（ローカルのメディア） | `public, max-age=31536000, immutable`（1 年） |
| `/media/proxy/{...}`（リモートのメディア） | `public, max-age=` インスタンス設定 `remote_media_cache_ttl` の秒数 |

どちらも `Cross-Origin-Resource-Policy: cross-origin` を付けて返すので、別オリジンのページが読める。

## 削除とキャッシュ

メディアを消しても、CDN のキャッシュは `max-age` の間残る。ローカルのメディアは最長 1 年、プロキシは `remote_media_cache_ttl` まで、削除したファイルが見え続けうる。
サーバーは CDN のキャッシュをパージしない。削除やモデレーションの直後に消したいときは、運営者が CDN の管理画面や API で手作業でパージする。

## Service Worker

Web クライアントの Service Worker は、別オリジンのメディアをキャッシュせず、ブラウザのネットワークに任せる。CDN のキャッシュだけが効く。
