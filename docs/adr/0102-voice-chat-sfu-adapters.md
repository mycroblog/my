# ADR-0102: 通話の SFU: LiveKit と Sora の adapter

- Status: Proposed
- Date: 2026-10-26
- Deciders: @rosylilly
- Issue: #1350

## Context（背景・課題）

M4.3（Voice Chat、epic #1345）は、通話の音声を P2P のほかに SFU で運べるようにする（FR-920）。対応する SFU は LiveKit（セルフホストと LiveKit Cloud）と Sora（時雨堂。ライセンスを買うセルフホストと Sora Cloud）である（#1345 の回答 6）。
ルームの振る舞いは [ADR-0100](0100-voice-chat-rooms.md)、方式に共通のポート `Transport` と P2P は [ADR-0101](0101-voice-chat-transport.md) が決めた。この ADR は、LiveKit と Sora がそのポートを満たす方法を決める。

事実は spike #1347 のレポート（[#1347 のコメント](https://github.com/mycroblog/my/issues/1347#issuecomment-6406319607)、確認日 2026-10-26）による。この ADR の第三者の事実は、断りがなければそのレポートの出典に基づく。

決定者の回答（#1345、原文）:

> 5\. 推奨: インスタンスで 1 つ（`voice = off / p2p / livekit / sora`）。全ルームが同じ方式 → これで OK
>
> 6\. SFU の接続先 → LiveKit のセルフホスト、LiveKit Cloud、Sora（ライセンス購入）、Sora Cloud のすべて
>
> 40\. 推奨: 聞き手は受信のみで入り、話者になるときに送受信で入り直す。1〜2 秒途切れる（要検証）が、聞くだけの人にマイクの許可を求めず、上りの帯域も使わない → これで OK
>
> 41\. 推奨: my は API の URL と付けるヘッダー（秘密）を設定で持ち、運営者が認証つきのリバースプロキシを置く。Webhook は HTTPS + Basic 認証を必須にする。Sora Cloud は署名と Bearer で守る。手順は運営者の文書に書く → これで OK
>
> 42\. 推奨: 参加の Webhook と 1 分ごとの照合で、禁止した人を見つけたらすぐ切る。数秒〜1 分ほど音が聞こえうることを運営者の文書に書き、厳密さが要るなら LiveKit Cloud を勧める → これで OK
>
> 43\. Sora の adapter の確かめ方 → **fake だけ**（CI は文書に沿った httptest の fake で契約テストを回し、本物での確認は利用する運営者に任せる。Sora の adapter は「実機未確認」と文書に書く）

## Decision Drivers（判断基準）

1. **決定者の回答をそのまま満たす**（#1345 の 5・6・40〜43）
2. **ルームのドメインが正**（ADR-0101 §2）: SFU は言われたことを強制し、見たことを知らせるだけ。SFU の Webhook は届くことを前提にしない
3. **サーバーレスで動く**: my から SFU への呼び出しは 1 回の HTTP で終わり、常時の接続を持たない（Cloud Run・Lambda）
4. **依存を小さく保つ**: AGPL-3.0-only と両立し、`CGO_ENABLED=0` で、バイナリと Web のバンドルを大きく増やさない
5. 運営者が使わない adapter をビルドから外せる（[ビルドタグ](../reference/build-tags.md) の既存の形）

## Considered Options（選択肢）

### サーバー側の呼び出し

- **G1**: SDK を使わず、`net/http` と自前の HS256 の JWT で、LiveKit の Twirp（HTTP + JSON）と Sora の HTTP API を呼ぶ
- **G2**: LiveKit の `server-sdk-go` を使う（Sora には Go のサーバー SDK がないので Sora は G1）

### ブラウザ側

- **W1**: `livekit-client` と `sora-js-sdk` を、通話の画面に入ったときだけ動的に読み込む
- **W2**: 両方の SFU のプロトコルを自前で書く
- **W3**: 静的に読み込む

## Decision（決定）

> 推奨案。Accepted にするのは人間の Decider のみ。

**G1・W1 を推奨する。**

### 1. 置き場所とビルド

- adapter は `internal/adapter/voice/livekit` と `internal/adapter/voice/sora`。Webhook の受け口は新しい `internal/interface/voice`（client API の OpenAPI に載せない。事業者が呼ぶ経路で、利用者は呼ばない。[ADR-0074](0074-payment-provider-port.md) の `internal/interface/payments` と同じ扱い）。
- ビルドタグ `no_voice_livekit`・`no_voice_sora` で外せる。外したビルドで `voice` にその値を設定すると、起動時に `config.ExcludedError` で止まる（`no_payments_stripe` と同じ形）。
- HTTP のクライアントは adapter が持つ専用のもの（タイムアウト 5 秒）。SFU は運営者が設定した宛先で、セルフホストの SFU はプライベートの IP にありうるので、SSRF の防御の `kernel.HTTPClient` は通さない（AGENTS.md の「運営者が設定した事業者の API」の規則）。`http.DefaultClient` は使わない。

### 2. サーバー側の呼び出し（G1）

- JWT は HS256 だけで、`crypto/hmac` と `encoding/json` で書く（数十行）。`server-sdk-go` は `CGO_ENABLED=0` でビルドできるが、RoomService のクライアントだけで 311 のパッケージ（pion/webrtc、moby/buildkit、prometheus など）を引き、バイナリが約 22 MB になる（spike の手元の計測）。G2 を採らないのはこのためである。
- LiveKit の Twirp は `POST <api_url>/twirp/livekit.RoomService/<Method>`、`Authorization: Bearer <JWT>`、JSON。Sora は `POST <api_url>/`（`x-sora-target: Sora_20xxxxxx.<Method>`）、Sora Cloud は `POST https://api.sora-cloud.shiguredo.app/sora-api` に `Authorization: Bearer <API キー>`。
- 契約テスト `testkit.VoiceTransportContract` を、LiveKit は `livekit-server --dev`（CI のコンテナ）と `httptest` の fake の両方で、Sora は `httptest` の fake だけで回す（回答 43）。fake は spike のレポートの要求と応答の形に沿う。

### 3. ポートの各操作

| 操作（ADR-0101 §2） | LiveKit | Sora |
| --- | --- | --- |
| `Capabilities` | `ServerEnforced`・`Events` | `ServerEnforced`・`ConnectCallback`・`Events` |
| `Open` | `CreateRoom`（名前はルームの ID、`max_participants` は `voice_max_participants`、`empty_timeout`・`departure_timeout` は短く）。冪等 | 何もしない（最初の接続でセッションができる） |
| `Grant` | アクセストークン（JWT、寿命 60 秒。`roomJoin`・`room`・`canSubscribe: true`・`canPublishData: false`。話者は `canPublish: true` と `canPublishSources: ["microphone"]`、聞き手は `canPublish: false`）と、クライアント向けの URL | my が署名した寿命 60 秒の参加の値（ルーム・セッション・役割）を `metadata` に入れさせる。Sora Cloud は加えて Cloud の `access_token`（JWT、HS256、`channel_id`・`role`・`exp`）。`Options` に `role`（`sendrecv` / `recvonly`）と `channel_id` |
| `SetSpeaking`（許可） | `UpdateParticipant` の `permission`。再接続なし（`reconnect` は偽） | 役割を変えられないので `reconnect` を真で返す。クライアントは `sendrecv` の新しい `Grant` で入り直す（回答 40） |
| `SetSpeaking`（取り消し） | `UpdateParticipant` で `canPublish: false`（公開中のトラックは自動で非公開になる） | 転送フィルター（そのコネクションの `audio` を `block`）ですぐ止め、`reconnect` を真で返して `recvonly` で入り直させる |
| `Mute` | `GetParticipant` でトラックを引き、`MutePublishedTrack(muted: true)` | 転送フィルターでそのコネクションの音声を `block` |
| `Unmute` | 何もしない（本人がクライアントでオンにする。要検証 §Open Questions） | 転送フィルターを消す |
| `Remove` | `RemoveParticipant` | `DisconnectConnection` |
| `Close` | `DeleteRoom` | `TerminateSession` |
| `Sessions` | `ListParticipants` | `ListChannelConnections` |

- **SFU の利用者の識別子**: ADR-0101 §2 のセッションの ID を使う（LiveKit の `identity`、Sora の `client_id`）。アカウントの ID は SFU に渡さない。同じアカウントの別の端末は別のセッションで、my が前のセッションを `Remove` する（ADR-0100 §5）。
  禁止や退出の後に古いトークンで入ってきた接続は、識別子が生きたセッションでないので、§4 の照合で切れる。
- **音声だけ**: LiveKit はトークンの `canPublishSources: ["microphone"]`、Sora は認証 Webhook の応答の `video: false`・`audio: true`・`audio_codec_type: "OPUS"`。運営者の文書は、セルフホストの LiveKit の `room.enabled_codecs` を音声だけにし、`room.auto_create` を `false` にすることを勧める（my の外でルームを作らせない）。
- **SFU の TURN**: SFU のときの STUN / TURN は SFU が持つもの（LiveKit の組み込みの TURN、Sora の TURN）を使い、`Grant.ICE` は空、`voice_stun_urls`・`voice_turn*`・`voice_ice_transport_policy` は使わない（起動時に設定されていれば警告）。
  SFU の構成では利用者の IP は SFU にだけ見え、他の参加者には見えない。

### 4. 参加の認可・イベント・照合

- **Sora の認証 Webhook**（`ConnectAuthorizer`）: `POST /voice/webhooks/sora/auth`。Sora が接続のたびに呼び、my は参加の値の署名と期限、セッションが生きていること、ルームで禁止されていないこと、`role` が `sendrecv` なら話者であること（ADR-0100 §4）、人数の上限を確かめ、`allowed` と §3 の音声の設定と `client_id` を返す。
  拒むときは `{"allowed": false, "reason": …}`（100 バイトまで）。応答の既定のタイムアウトは 5 秒（Sora Cloud は 10 秒）なので、判定は投影と KV の読み出しだけにし、外部の I/O をしない。冷えた起動で間に合わないことがある（運営者の文書）。
- **イベント**（`EventSource`）: LiveKit は `POST /voice/webhooks/livekit`（`Authorization` の JWT を API シークレットで検証し、JWT の `sha256` と本文の SHA-256 を定数時間で比べる）。Sora は `POST /voice/webhooks/sora/events`。
  検証した参加・退出・ルームの終了は、イベントの `id` を冪等キーにした outbox のジョブ `voice.media_event` に積む。ジョブは、参加が生きたセッションでなければ（禁止・退出の後・未知）`Remove` し、退出ならそのセッションのハートビートの値を消して ADR-0100 §6 の切断として扱う。受信の行は DB に残さない（イベントは照合を早めるだけ。ADR-0101 §2）。
- **照合**: ADR-0100 §6 の `voice.sweep`（1 分ごと）が、開いたルームごとに `Sessions` を呼ぶ（ADR-0101 §2）。開いたルームの数は運営者の上限で抑えられ、呼び出しは 1 ルーム 1 回。
- **セルフホストの LiveKit の再入室**: `RemoveParticipant` はセルフホストではトークンを失効させず、接続中のクライアントは 5 分ごとに寿命 10 分のトークンを受け取る（LiveKit のサーバーの実装）。退出させた人が最大でおよそ 10 分、入り直せる。
  入ってきたら参加の Webhook（`participant_joined`）か 1 分ごとの照合で見つけ、すぐ切る。その間、数秒から 1 分ほど音が聞こえうることを運営者の文書に書き、厳密さが要るなら LiveKit Cloud を勧める（回答 42。LiveKit Cloud の `RemoveParticipant` はトークンをすぐ失効させる）。

### 5. 設定と守り方（回答 41）

ADR-0101 §1 の表に足す。秘密は `_FILE` でも渡せ、TOML には書けない。

| キー | 意味 |
| --- | --- |
| `voice_livekit_url` | クライアントが繋ぐ URL（`wss:`） |
| `voice_livekit_api_url` | my が呼ぶ API の URL（既定は `voice_livekit_url` の `https:`） |
| `voice_livekit_api_key` / `voice_livekit_api_secret` | API キーとシークレット（シークレットは秘密）。アクセストークンの署名と Webhook の検証に使う |
| `voice_sora_mode` | `self_hosted` / `cloud` |
| `voice_sora_signaling_urls` | クライアントが繋ぐシグナリングの URL の一覧（`wss:`） |
| `voice_sora_api_url` | `self_hosted`: my が呼ぶ API の URL（運営者の認証つきのリバースプロキシ） |
| `voice_sora_api_header_name` / `voice_sora_api_header_value` | `self_hosted`: API の呼び出しに付けるヘッダー（値は秘密。例 `Authorization: Basic …`） |
| `voice_sora_webhook_username` / `voice_sora_webhook_password` | `self_hosted`: Sora の Webhook の Basic 認証（パスワードは秘密）。必須 |
| `voice_sora_cloud_project_id` / `voice_sora_cloud_api_key` | `cloud`: プロジェクトの ID と API キー（秘密）。`access_token` の署名、API の Bearer、Webhook の署名（`sora-cloud-signature`、HMAC-SHA256）の検証に使う |
| `voice_join_secret` | Sora の参加の値（§3）の署名の秘密。`voice = sora` で必須 |

- セルフホストの Sora の HTTP API には認証がなく、ループバックだけで受ける設定が勧められている。my はそのまま呼ばず、運営者が API の前に認証つきのリバースプロキシを置き、my はそのヘッダーを付ける（回答 41）。Webhook は HTTPS と Basic 認証を必須にし、どちらかが欠けた設定では起動しない。
  Lambda の Function URL と Cloud Run は受け側で mTLS を終端できないので、mTLS は使わない。
- Sora Cloud のチャネルの ID は `<ルームの ID>@<プロジェクトの ID>`。

### 6. ブラウザの SDK（W1）

- `livekit-client`（Apache-2.0）と `sora-js-sdk`（Apache-2.0）を `app/web` の依存に足す。[ADR-0025](0025-web-client-stack.md) の技術スタックへの追加はこの ADR が決める。
- 大きさ（spike の手元の計測、`bun build --minify`、gzip -9）: `livekit-client` 約 586 KB（gzip 150 KB）、`sora-js-sdk` 約 49 KB（gzip 11 KB）。通話の画面に入り、`Grant.Kind` が分かってから、その方式の SDK だけを動的な `import()` で読み込む。
  他の画面のバンドルは増えない。
- SDK を使うコードは ADR-0101 §7 の `features/voice/rtc/` に方式ごとのファイルで置き、P2P と同じ内部の境界（接続・話す・ミュート・入り直す・切る・参加者の音声の再生）に包む。
- W2 を採らないのは、LiveKit と Sora のシグナリングのプロトコルを自前で保つ費用が大きく、版の追従が要るからである。W3 を採らないのは、通話を使わない利用者にも 150 KB（gzip）を読ませるからである。

### 7. 運営者の文書

ADR-0101 §9 の通話の文書に足す:

- 方式の選び方: 小さなインスタンスには P2P か LiveKit Cloud の無料のプラン（Build: $0、月 5,000 分・50 GB・同時 100 接続。確認日 2026-10-26、[LiveKit の料金](https://livekit.com/pricing)）。LiveKit と Sora は UDP の常駐のプロセスなので、Cloud Run・Lambda ではなく別の VM で動かす（要検証）
- LiveKit: セルフホストの設定（`auto_create: false`・`enabled_codecs`・Webhook の URL `/voice/webhooks/livekit`）、`livekit-server --dev` での試し方、セルフホストの再入室の制限（§4）
- Sora: リバースプロキシと Basic 認証の置き方、Webhook の URL（`/voice/webhooks/sora/auth`・`/voice/webhooks/sora/events`）、認証 Webhook のタイムアウトと冷えた起動、聞き手が話者になるときの途切れ（§3）。
  料金とライセンスの形（Sora Cloud は月の基本料 5,000 円から、無料の期間はない。パッケージは同時接続数と期間の前払い。製品のページは海外の法人・団体・個人には提供していないと書く）。
  **Sora の adapter は実際の Sora で確かめていない**（文書に沿った fake でだけ試している。回答 43）

この決定を覆す条件: LiveKit の Twirp か Sora の HTTP API が、自前の呼び出しでは追いにくい頻度で変わると分かったとき（G2 か、生成したクライアントにする）。Sora が接続の途中で役割を変える API を出したとき（§3 の入り直しをやめる）。
セルフホストの LiveKit がトークンを失効させる手段を持ったとき（§4 の制限を外す）。

## Consequences（影響）

- 良い影響: どちらの SFU も 1 回の HTTP の呼び出しだけで動かせ、my はサーバーレスのまま SFU を使える。サーバーの依存は増えない。
- 良い影響: 通話を使わない画面のバンドルは増えない。SFU のときは利用者の IP が他の参加者に見えず、発話の制限を SFU が強制する（P2P の結託の限界がない）。
- 悪い影響: Sora では聞き手が話者になるとき、話者が聞き手に戻るときに入り直しで音が途切れる（要検証: 1〜2 秒）。
- 悪い影響: セルフホストの LiveKit では、退出させた人が切られるまで数秒から 1 分ほど聞こえうる。
- 悪い影響: Sora の adapter は実機で確かめず、Sora の API の変化に気づくのが運営者の報告になる。セルフホストの Sora は、運営者がリバースプロキシを置く手間を負う。
- 悪い影響: Sora の認証 Webhook は同期で、my の冷えた起動が参加の失敗になりうる。
- 後続の作業（ADR の決定の後に Issue にする）:
  - `internal/adapter/voice/livekit`（Twirp・JWT・Webhook の検証）と契約テスト（`livekit-server --dev` と fake）
  - `internal/adapter/voice/sora`（self_hosted・cloud の API・参加の値・認証 Webhook・転送フィルター・Webhook の検証）と fake の契約テスト
  - `internal/interface/voice` の Webhook の受け口、`voice.media_event`、`voice.sweep` の照合
  - 設定のキーと起動時の検査、ビルドタグ（`docs/reference/build-tags.md`）
  - Web の `features/voice/rtc/` の LiveKit・Sora の実装と動的な読み込み、依存の追加
  - 運営者の文書（§7）

## Pros and Cons of the Options（詳細比較・任意）

| | G1（自前の HTTP） | G2（`server-sdk-go`） |
| --- | --- | --- |
| 依存 | なし | 311 パッケージ、バイナリ約 22 MB |
| 書く量 | JWT・Twirp の呼び出し・Webhook の検証（数百行） | 少ない |
| 版の追従 | 自分で追う | SDK の更新で追う |
| Sora との揃い | 同じ形 | Sora は結局 G1 |

## Open Questions / 要検証

- LiveKit の `MutePublishedTrack(muted: true)` の後、本人がクライアントでマイクをオンにできるか（要検証: `livekit-server --dev`。できなければ `Unmute` で `UpdateParticipant` を使って発話の許可を付け直す）
- Sora の転送フィルターで、特定の送り手の音声をチャネルの全員に対して止める規則の書き方（要検証: 規則の `connection_id` が送り手を指すか）
- Sora で同じ `client_id` のまま入り直すときの扱いと、途切れの長さ（要検証: 実機を持つ運営者の報告を待つ）
- LiveKit のセルフホストの Webhook の再送の回数と間隔（要検証）
