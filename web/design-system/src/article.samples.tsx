// SPDX-License-Identifier: AGPL-3.0-only

// The text of a sample article (ADR-0160) in en and ja, with every block the article's allowlist keeps: headings h2
// to h6, paragraphs with emphasis, inline code and links, lists, a quote, a code block with a long line, a table, an
// image of the author's own upload, a link that stands for an image the server did not keep, and a rule.

import { ArticleTable } from "./article";
import { ContentLink } from "./content";

// A wide picture of the author's own upload: a window at dusk over a town's roofs.
const dusk = `data:image/svg+xml,${encodeURIComponent(
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 150" width="960" height="600"><defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="#34405f"/><stop offset=".6" stop-color="#b9786a"/><stop offset="1" stop-color="#eab486"/></linearGradient></defs><rect width="240" height="150" fill="url(#s)"/><circle cx="160" cy="96" r="18" fill="#ffe2b0"/><path d="M0 112 H30 V96 H52 V108 H78 V90 H104 V112 H130 V100 H158 V114 H186 V94 H214 V110 H240 V150 H0Z" fill="#3a2c35"/></svg>`,
)}`;

// A code line longer than the column, with a break opportunity where the content renderer marks one (after a slash or
// a question mark, before a pipe: app-components' post-content.tsx), so that it wraps there first.
const longLine =
  "docker run --rm -e MY_DOMAIN=my.example -e MY_DATABASE_URL=postgres://my:secret@db/my -p 8080:8080 ghcr.io/mycroblog/my:latest serve"
    .split(/(?<=[/?])(?=[^/])|(?<=.)(?=\|)/g)
    .flatMap((part, i) => (i === 0 ? [part] : [<wbr key={i} />, part]));

const en = (
  <>
    <p>
      We opened the server to twelve friends and a cat. By the end of the year, two hundred people called it home. This
      is what running it <em>actually</em> took, and what we would do again.
    </p>
    <h2>The first month</h2>
    <p>
      Nobody reads the rules on day one. We wrote three, pinned them, and linked them from the{" "}
      <ContentLink href="https://my.example/about" kind="link">
        about page
      </ContentLink>
      . What helped more was a weekly post that said, in plain words, what had changed.
    </p>
    <ul>
      <li>Answer every report within a day, even if only to say you saw it.</li>
      <li>Keep the sign-up open, but approve each account by hand.</li>
      <li>
        Run <code>my admin backup</code> before every upgrade.
      </li>
    </ul>
    <h3>What the costs were</h3>
    <ArticleTable>
      <thead>
        <tr>
          <th>Item</th>
          <th align="right">Per month</th>
          <th>Note</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>Server</td>
          <td align="right">$12</td>
          <td>Two cores, four gigabytes</td>
        </tr>
        <tr>
          <td>Object storage</td>
          <td align="right">$3</td>
          <td>Media and backups</td>
        </tr>
        <tr>
          <td>Domain</td>
          <td align="right">$1</td>
          <td>Paid yearly</td>
        </tr>
      </tbody>
    </ArticleTable>
    <blockquote>
      <p>A small server is a neighbourhood, not a product. People stay for the neighbours.</p>
    </blockquote>
    <img src={dusk} alt="The view from the office window at dusk" width={960} height={600} loading="lazy" />
    <h3>Upgrading without fear</h3>
    <ol>
      <li>Read the release notes.</li>
      <li>Take a backup.</li>
      <li>Pull the new image and restart.</li>
    </ol>
    <pre>
      <code>{longLine}</code>
    </pre>
    <h4>When it goes wrong</h4>
    <p>
      Roll back to the backup, then ask in the project&apos;s forum. A screenshot of the log says more than a
      description of it:{" "}
      <ContentLink href="https://images.example.net/log.png" kind="link">
        the log on the night of the outage
      </ContentLink>
      .
    </p>
    <h5>A note on federation</h5>
    <p>Other servers will cache your posts. Deleting one asks them to forget it; most do.</p>
    <h6>Thanks</h6>
    <p>To everyone who reported a bug instead of leaving.</p>
    <hr />
    <p>Next year: a second moderator, and a proper status page.</p>
  </>
);

const ja = (
  <>
    <p>
      {
        "十二人の友だちと一匹の猫のためにサーバーを開きました。一年が終わるころには、二百人がここを居場所にしていました。"
      }
      運営に<em>実際に</em>必要だったこと、もう一度やるならどうするかを書きます。
    </p>
    <h2>最初の一か月</h2>
    <p>
      初日にルールを読む人はいません。三つだけ書いて固定し、
      <ContentLink href="https://my.example/about" kind="link">
        サーバーについてのページ
      </ContentLink>
      から案内しました。それより役に立ったのは、何が変わったかを毎週ふつうの言葉で知らせる投稿でした。
    </p>
    <ul>
      <li>通報には一日以内に答える。見ました、のひとことでも。</li>
      <li>登録は開けておき、アカウントは一件ずつ手で承認する。</li>
      <li>
        更新の前には必ず <code>my admin backup</code> を実行する。
      </li>
    </ul>
    <h3>かかった費用</h3>
    <ArticleTable>
      <thead>
        <tr>
          <th>項目</th>
          <th align="right">月額</th>
          <th>メモ</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>サーバー</td>
          <td align="right">1,800円</td>
          <td>2 コア、メモリ 4 GB</td>
        </tr>
        <tr>
          <td>オブジェクトストレージ</td>
          <td align="right">450円</td>
          <td>メディアとバックアップ</td>
        </tr>
        <tr>
          <td>ドメイン</td>
          <td align="right">150円</td>
          <td>年払い</td>
        </tr>
      </tbody>
    </ArticleTable>
    <blockquote>
      <p>小さなサーバーは製品ではなく、ご近所です。人はご近所さんがいるから残ります。</p>
    </blockquote>
    <img src={dusk} alt="夕暮れの事務所の窓からの眺め" width={960} height={600} loading="lazy" />
    <h3>こわがらずに更新する</h3>
    <ol>
      <li>リリースノートを読む。</li>
      <li>バックアップを取る。</li>
      <li>新しいイメージを取ってきて再起動する。</li>
    </ol>
    <pre>
      <code>{longLine}</code>
    </pre>
    <h4>うまくいかなかったら</h4>
    <p>
      バックアップに戻してから、プロジェクトのフォーラムで聞きましょう。ログは説明よりスクリーンショットのほうが伝わります：
      <ContentLink href="https://images.example.net/log.png" kind="link">
        障害の夜のログ
      </ContentLink>
      。
    </p>
    <h5>連合について</h5>
    <p>ほかのサーバーはあなたの投稿をキャッシュします。消すと忘れるよう頼みますし、たいていは忘れてくれます。</p>
    <h6>謝辞</h6>
    <p>去るかわりにバグを報告してくれたみなさんへ。</p>
    <hr />
    <p>来年は、二人目のモデレーターと、ちゃんとした状態ページを。</p>
  </>
);

/** The text of the sample article in `lang` (en or ja). */
export const articleText = { en, ja } as const;

/** The cover of the sample article: a sea under a pale sun, with the blurhash the server would give it. */
export const articleCover = {
  src: `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 90" width="640" height="360"><rect width="160" height="90" fill="#7fb3d5"/><circle cx="112" cy="34" r="12" fill="#fffbe8"/><path d="M0 60 L40 44 L80 56 L120 40 L160 52 V90 H0Z" fill="#3f6f86"/></svg>`,
  )}`,
  width: 640,
  height: 360,
  alt: "A blue sea under a pale sun",
  blurhash: "IxHWWH%h01",
};
