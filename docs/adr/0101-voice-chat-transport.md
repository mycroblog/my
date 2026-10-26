# ADR-0101: 通話の接続: 方式のポート・P2P のシグナリング・STUN / TURN・ルームのストリーム

- Status: Proposed
- Date: 2026-10-26
- Deciders: @rosylilly
- Issue: #1349

## Context（背景・課題）

M4.3（Voice Chat、epic #1345）のルームの振る舞いとデータモデルは [ADR-0100](0100-voice-chat-rooms.md) が決めた。この ADR は、音声をどう運ぶかのうち、方式に共通の部分と P2P を決める（FR-920・FR-921）。
SFU（LiveKit・Sora）の adapter の細部は ADR-C（#1350）が、spike #1347 のレポートの後に決める。

ADR-0100 がこの ADR に任せたもの: 設定 `voice` と `off` の振る舞い（§2）、ルームのストリームとハートビートの経路（§6）、マイクのオン・オフとミュートさせる操作の配り方（§5）、運営者の上限（§5・§6）。

決定者の回答（#1345、原文）のうち、この ADR の決定に入るもの:

> 3\. P2P のときのルームの人数の上限 → **運営者が設定**（既定 10）
>
> 4\. 推奨: STUN/TURN の URL と TURN の共有シークレット（coturn の REST 認証・Cloudflare TURN 等）を運営者が設定し、IP を隠すため TURN 経由だけにする設定も持つ → これで OK
>
> 5\. 推奨: インスタンスで 1 つ（`voice = off / p2p / livekit / sora`）。全ルームが同じ方式 → これで OK
>
> 7\. 推奨: シグナリングは my を通るので、許可のない人の音声トラックを含む offer をサーバーが中継しない。改造クライアントへの完全な防御ではないことを文書に書く → これで OK
>
> 20\. 推奨: 1 ルームの最長時間・同時に開けるルーム数・ルームの人数（SFU）を運営者が設定でき、既定はなし（P2P の人数は別に設定、既定 10） → これで OK
>
> 29\. 推奨: 既存の SSE（ADR-0050 の Bus）と同じ仕組みでルーム用のストリームを開き、送る側は POST。Lambda Function URL・Cloud Run でも動く（Lambda は 15 分ごとに再接続） → これで OK

## Decision Drivers（判断基準）

1. **決定者の回答をそのまま満たす**（#1345 の 3〜5・7・20・29）
2. **ほぼ $0 の構成で動く**: Cloud Run・Lambda（Function URL + Lambda Web Adapter）・1 プロセスの SQLite で、WebSocket の別の基盤や常駐のメディアサーバーを要らないようにする（P2P のとき）
3. **ルームのドメインは方式を知らない**: ADR-0100 の操作（参加・発話の許可と取り消し・ミュートさせる・退出させる・閉じる）を 1 つのポートで表し、P2P と SFU の adapter が満たす
4. **利用者の IP アドレスを守れる**: 運営者が望めば、相手に IP を見せない構成にできる
5. **既存のストリームの約束を崩さない**（[ADR-0050](0050-stream-pubsub-across-processes.md)）: `Bus` の知らせは 1 KiB まで、内容を載せない、重複と順序の入れ替わりがありうる
6. 改造されたクライアントからの防御は、サーバーが見えるもの（シグナリング）の範囲で行い、できないことを文書に書く

## Considered Options（選択肢）

### ルームのストリームの経路

- **R1**: ルームごとの SSE `GET /voice/rooms/{id}/stream` を、参加の後に別に開く
- **R2**: いまの `GET /streaming` に通話のイベントを混ぜる

### シグナリングの本文の運び方

- **B1**: 本文は `KeyValueStore` に短い期限で置き、`Bus` には ID だけを流す
- **B2**: 本文を `Bus` の知らせにそのまま載せ、通話の知らせだけ上限を広げる

### P2P の張り方

- **P1**: 少なくとも一方が話者である組だけを結ぶ（聞き手どうしは結ばない）
- **P2**: 参加者のすべての組を結ぶ（フルメッシュ）

## Decision（決定）

> 推奨案。Accepted にするのは人間の Decider のみ。

**R1・B1・P1 を推奨する。**

### 1. 設定（回答 3・4・5・20）

デプロイの設定（[設定のリファレンス](../reference/configuration.md)、環境変数 `MY_*`。秘密は `_FILE` でも渡せ、TOML には書けない）に足す。

| キー | 値 | 既定 |
| --- | --- | --- |
| `voice` | `off` / `p2p` / `livekit` / `sora` | `off` |
| `voice_p2p_max_participants` | P2P のルームの人数の上限（2〜50） | `10` |
| `voice_max_participants` | SFU のルームの人数の上限（0 は上限なし） | `0` |
| `voice_max_room_duration` | 1 ルームの最長時間（0 は上限なし） | `0` |
| `voice_max_open_rooms` | 同時に開けるルームの数（0 は上限なし） | `0` |
| `voice_stun_urls` | STUN の URL の一覧（`stun:` / `stuns:`） | なし |
| `voice_turn` | `none` / `coturn` / `cloudflare` | `none` |
| `voice_turn_urls` | coturn の TURN の URL の一覧（`turn:` / `turns:`） | なし |
| `voice_turn_secret` | coturn の `static-auth-secret`（秘密） | なし |
| `voice_turn_cloudflare_key_id` / `voice_turn_cloudflare_api_token` | Cloudflare の TURN キーの ID と API トークン（トークンは秘密） | なし |
| `voice_turn_credential_ttl` | TURN の資格情報の有効期間 | `6h` |
| `voice_ice_transport_policy` | `all` / `relay`（TURN 経由だけ） | `all` |

- `voice = off` のとき、通話の API は `404` を返し、Web は通話の画面と入口を出さない（ADR-0100 §2）。起動時に他の `voice_*` は検査しない。
- 起動時の検査（失敗したら起動しない）: `voice_ice_transport_policy = relay` で `voice_turn = none` は誤り（TURN がないと繋がらない）。`voice_turn = coturn` で URL か秘密がない、`cloudflare` でキーかトークンがないのも誤り。
  `voice = p2p` で STUN も TURN もないときは警告をログに出す（同じ LAN の中でしか繋がらない）。
- SFU（`livekit` / `sora`）のときの `voice_stun_urls`・`voice_turn*`・`voice_ice_transport_policy` の扱いと、SFU の接続先の設定は ADR-C が決める。
- 上限は §1 の設定で、ADR-0100 §5・§6 が評価する（参加の `409`、`voice.sweep` の `time_limit`、開く操作の `409`）。

### 2. 方式のポート（判断基準 3）

`internal/app/voice` に置く。型は標準ライブラリだけで、SDK の型を出さない。adapter は `internal/adapter/voice/<name>`（`p2p`、ADR-C の `livekit`・`sora`、テスト用の `fake`）。契約は `testkit.VoiceTransportContract`。
形は spike #1347 のレポート（[ポートの形の案](https://github.com/mycroblog/my/issues/1347#issuecomment-6406319607)）の差を吸収するように決める。

```go
// Transport carries the audio of the rooms (ADR-0101 §2). The room domain is the source
// of truth; a transport enforces what it is told and reports what it saw.
type Transport interface {
    Capabilities() Capabilities
    // Open prepares the media path of a room. Idempotent.
    Open(ctx context.Context, room RoomRef) error
    // Grant returns what the client of a participant session needs to connect.
    // canSpeak is whether the session may publish audio now.
    Grant(ctx context.Context, room RoomRef, session Session, canSpeak bool) (Grant, error)
    // SetSpeaking allows or stops the audio of a session. Reconnect reports that the
    // client must connect again with a new grant for the change to take effect.
    SetSpeaking(ctx context.Context, room RoomRef, session Session, canSpeak bool) (reconnect bool, err error)
    // Mute stops the audio of a session; Unmute lifts it when the participant turns the
    // microphone on again.
    Mute(ctx context.Context, room RoomRef, session Session) error
    Unmute(ctx context.Context, room RoomRef, session Session) error
    // Remove disconnects a session from the media path.
    Remove(ctx context.Context, room RoomRef, session Session) error
    // Close ends the media path of a room. Idempotent.
    Close(ctx context.Context, room RoomRef) error
    // Sessions returns the sessions the media path sees now, for reconciliation.
    Sessions(ctx context.Context, room RoomRef) ([]SessionRef, error)
}

type Capabilities struct {
    ServerEnforced  bool // false for p2p: speaking is enforced by the relay and the peers (§4)
    ConnectCallback bool // the media server asks before every connect (ConnectAuthorizer)
    Events          bool // the media server sends events (EventSource)
}

type Grant struct {
    Kind      string            // "p2p", "livekit", "sora"
    URL       string            // the SFU endpoint; empty for p2p
    Token     string            // the SFU access token; empty for p2p
    Options   map[string]string // method-specific options for the client (ADR-C)
    ICE       []ICEServer       // STUN / TURN servers with short-lived credentials
    RelayOnly bool              // iceTransportPolicy = "relay"
    ExpiresAt time.Time         // the client asks for a new grant before this
}

// ConnectAuthorizer is implemented by a transport whose media server asks before every
// connect; decide applies the room domain (bans, roles, one room per account).
type ConnectAuthorizer interface {
    AuthorizeConnect(ctx context.Context, header http.Header, body []byte, decide func(ConnectAttempt) ConnectDecision) (status int, response []byte, err error)
}

// EventSource is implemented by a transport whose media server sends events.
type EventSource interface {
    // ParseEvents verifies and decodes one delivery of events.
    ParseEvents(ctx context.Context, header http.Header, body []byte) ([]Event, error)
}
```

- **セッション**: 参加（ADR-0100 の参加の行）ごとに、端末が参加するたびに新しい不透明なセッションの ID を作る。同じアカウントが別の端末で参加すると、セッションが替わり、前のセッションのストリームは `replaced` を受けて閉じ、`Remove` で前の接続を切る（ADR-0100 §5）。
  セッションの ID は `KeyValueStore` のハートビートの値のキーで、DB には書かない。SFU の利用者の識別子にセッションとアカウントのどちらを使うかは ADR-C が決める。
- **呼び出しの時機**: ポートの呼び出しは外部の I/O になりうるので、トランザクションの中では呼ばない。ルームの変更をコミットしたら、outbox のジョブ `voice.apply`（冪等。キーは（ルーム, 変更のイベント））が `Open`・`SetSpeaking`・`Mute`・`Remove`・`Close` を呼ぶ（[ADR-0004](0004-job-queue-abstraction.md)）。
  `Grant` は参加と更新の要求の中で、`Unmute` はマイクの操作（§6）の中で、トランザクションの外で呼ぶ。
- **再接続**: `SetSpeaking` が `reconnect` を返したら（接続の途中で役割を変えられない方式）、`voice.apply` はそのセッションのストリームに新しい `Grant` を `grant` のイベント（`reconnect: true`）で送り、クライアントは入り直す。
- **照合**: `Sessions` は ADR-0100 §6 の `voice.sweep` が開いたルームごとに呼び、ルームのドメインにいてメディアの経路にいないセッションを退出させ（ハートビートが生きていても）、メディアの経路にいてドメインで退出・禁止のセッションを `Remove` する。
  イベント（`EventSource`）は照合を早めるだけで、届くことを前提にしない。受け口とイベントの記録は ADR-C が決める。
- **P2P の adapter**: `Capabilities` はすべて偽。`Grant` は §5 の ICE の設定だけを返す。`Open`・`SetSpeaking`（`reconnect` は偽）・`Mute`・`Unmute`・`Remove`・`Close` はメディアを持たないので何もしない。`Sessions` はハートビートの値を返す。
  P2P の強制は §4 のシグナリングの中継と §3 のストリームのイベントが行う。
- adapter の失敗は `voice.apply` が再試行する。再試行を尽くしたら dead job に残し、ルームの状態（DB）は変えない（ルームのドメインが正で、メディアの経路が後から従い、`voice.sweep` の照合が残りを直す）。

### 3. ルームのストリーム（回答 29、R1）

- `GET /voice/rooms/{id}/stream?session=…`: Server-Sent Events。いまの `GET /streaming` と同じく手で書き（[ADR-0029](0029-api-code-generation.md)）、`fetch` のストリームで `Authorization: Bearer` を送る。スコープは `read:voice`。
  開けるのは、そのセッションが参加中で生きている（ハートビートの値がある）ときだけ。それ以外は `404`。1 セッションに 1 本で、同じセッションで開き直すと前の 1 本を閉じる。
- イベント:

  | `event` | `data` | 宛先 |
  | --- | --- | --- |
  | `state` | ルームと参加者の一覧（`VoiceRoom`。ADR-0100 §14 の `GET` と同じ形） | 全員 |
  | `mic` | `{participant, on}` | 全員 |
  | `reaction` | `{participant, emoji}` | 全員（送った人とブロックの関係にある人を除く。ADR-0100 §10） |
  | `signal` | `{from, kind, sdp?, candidate?}`（§4） | 宛先のセッション |
  | `muted` | `{by}` | ミュートされたセッション |
  | `grant` | `{grant, reconnect}`（話せるようになって方式が入り直しを要するときなど。§2） | そのセッション |
  | `removed` / `replaced` / `ended` | `{reason}`。送った後に閉じる | そのセッション / 全員 |

- 開くと `: connected` と最初の `state` を送り、その後 15 秒ごとに `: heartbeat` のコメント行を送る。ハートビートのたびにアクセストークンを照合し直す（いまのストリームと同じ）。
- **プロセスをまたぐ**: いまの `stream.Hub` にルームの購読（キーはルームの ID）を足し、`stream.Relay` の `Bus` で運ぶ。知らせは 1 KiB 以下の JSON:
  - `{"v":1,"o":…,"k":"voice_room","r":"<room>"}`: 受けたプロセスはルームの状態をユースケースで読み直し、自分のプロセスの購読者に `state` を送る（閲覧者ごとのブロックの印は ADR-0100 §10 のとおり読み直しで付ける）
  - `{"k":"voice_signal","r":…,"m":"<message id>"}`: §4 の本文を `KeyValueStore` から読み、宛先のセッションを持つプロセスだけが送る
  - `{"k":"voice_mic","r":…,"p":"<participant>","on":true}`、`{"k":"voice_reaction","r":…,"p":…,"e":"<emoji>"}`、`{"k":"voice_mute","r":…,"s":"<session>","b":"<by>"}`
  - 通話の知らせは、内容を載せない ADR-0050 §1 の規則の例外として、マイクの状態・リアクションの絵文字（Unicode か `:shortcode:`、64 バイトまで）・ミュートした人を載せる。どれも DB にない一時の事実で、読み直す先がないからである。
    受けたプロセスは送る前に、宛先がそのルームの参加者であることと、ブロックの関係を確かめる
- `state` は 1 秒に 1 回までにまとめる（同じルームの `voice_room` の知らせが続いても、最後に読み直した状態を 1 回送る）。
- **再接続**: 切れたら、クライアントは同じセッションで開き直し、最初の `state` で追いつく。`Last-Event-ID` での再開はしない。ハートビートの値（45 秒、ADR-0100 §6）が切れる前に戻れば、参加は続く。
  Lambda の 15 分ごとの切断と Cloud Run のリクエストの時間切れは、この再接続で吸収する。
- R2 を採らないのは、`/streaming` の各接続が「いまどのルームにいるか」を知り、別のプロセスで受けた参加の要求に合わせて購読を替える必要があり、アカウントのストリームの上限（8）と通話の 1 本の規則も混ざるからである。
  代わりに、通話の間は 1 人につき SSE が 2 本になる（Lambda では実行環境を 2 つ占める。§8）。

### 4. P2P のシグナリング（回答 7、B1・P1）

- **結ぶ組（P1）**: 少なくとも一方が話者（ホスト・共同ホストで話している人を含む）である 2 つのセッションを結ぶ。聞き手どうしは結ばない。聞き手が話者になったら、まだ結んでいないすべての参加者と結び、話者が聞き手に戻ったら、聞き手どうしになった組を切る。
  人数の上限 `voice_p2p_max_participants`（既定 10）で、組の数は最大 45 に収まる。
- **offer する側**: 結ぶ組のうち、セッションの ID が大きい側が最初の offer を送る。再ネゴシエーションは WebRTC の perfect negotiation の手順に従い、ID が小さい側を polite とする。
- **送り方**: `POST /voice/rooms/{id}/signals`（`write:voice`）に `{session, to, kind, sdp?, candidate?}`（`kind` は `offer` / `answer` / `candidate` / `restart`）。サーバーは次を検査し、満たさなければ `422` で中継しない:
  - 送り手と宛先が同じルームの生きたセッションで、§4 の結ぶ組である
  - 本文は 16 KiB まで。セッションごとに 1 秒に 20 件まで（`KeyValueStore` のレート制限、`429`）
  - SDP の `m=` 行は `audio` だけ（`video`・`application` を拒む。音声のみ、データチャネルも使わない）
  - 送り手が話せない（ADR-0100 §4 の `CanSpeakFreely` が偽で、話者でない）とき、`m=audio` の向きは `recvonly` か `inactive` だけ。`sendrecv` / `sendonly` を含む offer と answer を拒む（回答 7）
- **運び方（B1）**: 検査を通った本文を `KeyValueStore` に 30 秒の期限で置き、`voice_signal` の知らせで ID を流す。宛先のセッションのストリームを持つプロセスが読み、`signal` として送る。届かなかった本文は期限で消える（クライアントは ICE の再起動でやり直す）。
  B2 を採らないのは、音声だけでも SDP は 1 KiB を超えることが多く、PostgreSQL の `NOTIFY` の上限（8,000 バイト未満）にも近づき、上限を広げると ADR-0050 のすべての adapter の前提が変わるからである。
- **受け手の側の強制**: クライアントは、最新の `state` で話者でない参加者から届く音声を再生しない。発話を取り消されたクライアントは送信のトラックを外して再ネゴシエーションする。
  サーバーの SDP の検査と受け手の側の再生の制御を合わせても、**改造したクライアントどうしが結託すれば防げない**（例: 改造した話者でない人と、改造した受け手）。これを運営者の文書に書き、厳密に強制したい運営者には SFU を勧める（回答 7）。
- `Mute`（ミュートさせる）は `muted` のイベントで本人のクライアントにマイクを切らせる。本人は再びオンにできる（ADR-0100 §5）。

### 5. STUN / TURN（回答 4）

- `Grant` の `ICE` は、`voice_stun_urls` と、`voice_turn` が発行する短命の資格情報の TURN を返す。資格情報の有効期間は `voice_turn_credential_ttl`（既定 6 時間）で、クライアントは `ExpiresAt` の前に `POST /voice/rooms/{id}/grant` で取り直し、`RTCPeerConnection.setConfiguration` で差し替える。
- **coturn**（`use-auth-secret`）: ユーザー名は `<期限の Unix 秒>:<セッションの ID>`、パスワードは `base64(HMAC(static-auth-secret, ユーザー名))`。サーバーの中で計算し、外部の I/O はない
  （確認日 2026-10-26: [coturn の turnserver.conf の例](https://github.com/coturn/coturn/blob/master/examples/etc/turnserver.conf) の `use-auth-secret`・`static-auth-secret` の説明。HMAC のハッシュ関数は既定の SHA-1 を使う。要検証: coturn の版ごとの既定）。
- **Cloudflare TURN**: `POST https://rtc.live.cloudflare.com/v1/turn/keys/{key_id}/credentials/generate-ice-servers`（`Authorization: Bearer <API トークン>`、本文 `{"ttl": 秒}`）が `iceServers` を返す。取り消しは `.../credentials/{username}/revoke`
  （確認日 2026-10-26: [Generate credentials](https://developers.cloudflare.com/realtime/turn/generate-credentials/)）。料金は SFU と使わないとき、クライアントへの送信 1 GB あたり $0.05（同日、[TURN service](https://developers.cloudflare.com/realtime/turn/)）。
  この呼び出しは運営者が設定した事業者の API なので、`internal/adapter/voice/turncloudflare` に置き、SSRF の防御の `kernel.HTTPClient` ではなく専用の HTTP クライアントを使う（AGENTS.md の規則）。`Grant` は参加の要求の中で呼び、失敗したら `503` を返す。
  同じセッションの資格情報は期限まで `KeyValueStore` に置いて使い回す。退出・退出させる操作で取り消す（`voice.apply`）。
- **IP を隠す（`relay`）**: `voice_ice_transport_policy = relay` のとき、`Grant.RelayOnly` を真にし、クライアントは `iceTransportPolicy: "relay"` で接続する。そのクライアント自身は TURN の候補しか出さないので、相手にはそのクライアントの IP が見えない。
  守られるのは設定に従うクライアントの IP で、改造したクライアントが自分の IP を出すことは防がない（それはそのクライアントの利用者の選択である）。
- TURN の運営の注意（運営者の文書）: coturn では内部のネットワークへの中継を `denied-peer-ip` で拒み、`external-ip` を設定する。TURN の転送量は事業者の料金になる。

### 6. 参加・ハートビート・マイクの API

ADR-0100 §14 の形に、接続のための操作を足す（細部は [client-api.md](../architecture/client-api.md) の手順で決める）。

- `POST /voice/rooms/{id}/participation` は `{session, grant}` を返す。クライアントはそのセッションでストリームを開く。
- `POST /voice/rooms/{id}/heartbeat`（`{session}`、15 秒ごと）: ADR-0100 §6 のハートビート。応答は `204`。ストリームを開いているだけではハートビートにしない（中継の装置がバッファしてストリームの書き込みが失敗しないことがあり、端末が生きていることを確かめられないため）。
- `POST /voice/rooms/{id}/mic`（`{session, on}`）: マイクのオン・オフ。DB に書かず、`voice_mic` の知らせで配る。ミュートさせられた後にオンにするときは、`Transport.Unmute` を呼んでから配る（§2）。
- `POST /voice/rooms/{id}/grant`（`{session}`）: 資格情報の取り直し（§5）。

### 7. Web の実装の置き場所と、ブラウザの前提

- `app/web/src/features/voice/` に置く（[ADR-0026](0026-client-workspace-and-component-architecture.md)）。WebRTC の接続を管理するコード（`RTCPeerConnection`・perfect negotiation・受け手の側の再生の制御・方式ごとの差）は React を使わない TypeScript として `features/voice/rtc/` に置き、
  `features/voice/hooks` のフックだけがそれを使う。`eslint-plugin-boundaries` の設定に `rtc` の要素を足し、`rtc` は `@my/ui`・`api`・`components` を import しない。
  SFU の方式のブラウザの SDK（依存の追加）は ADR-C が決める。P2P はブラウザの WebRTC の API だけを使い、依存を足さない。
- マイクは `getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })`。映像は求めない。HTTPS（secure context）が要る。
- 参加の操作（ボタンを押す）を利用者の操作として、その中で音声の再生を始める（ブラウザの自動再生の制限）。
- 対応するブラウザは Web クライアントの対応範囲のまま。iOS の PWA では、アプリを背面にすると音声が止まることがある（要検証: iOS の版ごとの挙動）。利用者の文書に書く。

### 8. 構成ごとの注意

| 構成 | 注意 |
| --- | --- |
| 1 プロセス（SQLite、`stream = hub`） | すべてプロセスの中で動く。`KeyValueStore` は `db` |
| 複数プロセス | `stream` を `postgres` / `valkey` / `sns` / `pubsub` にする（いまのストリームと同じ）。`stream = hub` で `role` が `all` 以外のときは、起動時の警告に通話を含める |
| Lambda | SSE の 1 本が実行環境 1 つを最大 15 分占める。通話の間は 1 人 2 本（§3）で、実行時間は参加者の数 × 時間で増える（[デプロイターゲット](../architecture/deployment-targets.md#ストリームプロセス間の知らせ)の見積もりの 2 倍） |
| Cloud Run | ストリームはリクエストで、開いている間 CPU が割り当てられる。リクエストの時間切れで切れたら §3 の再接続 |

- P2P では、音声はこのサーバーを通らない（TURN を使うときは TURN のサーバーを通る）。サーバーの負荷はシグナリング・ハートビート・ストリームだけである。

### 9. 運営者の文書と計測

- `docs/operations/` に通話の文書を足す: 方式の選び方、P2P の人数と回線の目安、STUN / TURN の設定（coturn・Cloudflare）、`relay` の意味、§4 の強制の限界、構成ごとの注意（§8）。`tools/agentkit/context-map.yaml` の `voice` の項目の `userdocs` にする。
- 計測（[ADR-0055](0055-service-observability.md)）: 開いたルームの数・参加者の数（Gauge）、中継したシグナリングと拒んだシグナリング（理由ごと）・TURN の資格情報の発行と失敗（Counter）。名前は ADR-0055 の規則で実装のときに決め、`agentkit docs inventory` の文書に載せる。

この決定を覆す条件: P2P の利用が上限の人数でも成り立たないと分かったとき（既定の人数を下げる）。`Bus` の adapter のいずれかで KV の読み直しが遅すぎて接続が間に合わないと分かったとき（B2 を通話の知らせに限って検討する）。
WebSocket を Lambda で安く使える形が出たとき（R1 のストリームと POST をまとめる）。

## Consequences（影響）

- 良い影響: P2P では新しい基盤なしで通話が動く。いまのストリームの仕組み（`Hub`・`Relay`・`Bus`）をそのまま使い、構成ごとの差がない。
- 良い影響: 方式のポートがルームのドメインと音声の経路を分け、SFU を後から足せる。ルームの状態は DB が正で、メディアの経路が後から従う。
- 良い影響: `relay` で、設定に従う利用者の IP を相手に見せない構成にできる。
- 悪い影響: P2P の発話の制限は、結託した改造クライアントを防げない（§4）。文書で示し、厳密さが要る運営者には SFU を勧める。
- 悪い影響: 通話の間は SSE が 1 人 2 本になり、Lambda の実行時間と接続の数が増える。
- 悪い影響: シグナリングの本文を KV に置くので、`KeyValueStore = db` では接続のたびに数十行の書き込みが起きる（短い期限で消える）。
- 悪い影響: SFU では `voice.apply` のジョブを通るので、発話の取り消し・退出させる操作がメディアの経路に効くまでキューの遅れが乗る。
- 後続の作業（ADR の決定の後に Issue にする）:
  - 設定のキーと起動時の検査、`Transport` のポート・契約テスト・`fake` と `p2p` の adapter、`voice.apply`
  - `stream.Hub` のルームの購読と通話の知らせ、ルームのストリーム、シグナリングの中継と SDP の検査、ハートビート・マイク・資格情報の API
  - TURN の資格情報（coturn・Cloudflare）
  - Web の `features/voice/rtc`（P2P の接続、perfect negotiation、受け手の側の制御）と通話の画面
  - 運営者の文書、計測、`context-map.yaml` の `voice` の項目、overview（ポートの表・§4.4）と deployment-targets の更新

## Pros and Cons of the Options（詳細比較・任意）

| | R1（ルームの SSE） | R2（`/streaming` に混ぜる） |
| --- | --- | --- |
| 接続の数 | 通話の間 1 人 2 本 | 1 本 |
| 購読の切り替え | 開くときに決まる | 別のプロセスの参加に合わせて替える必要がある |
| 上限・認可 | 通話だけの規則（1 セッション 1 本） | アカウントの上限（8）と混ざる |
| 切れたときの影響 | 通話だけ | ホームと通知も読み直す |

| | P1（話者を含む組だけ） | P2（すべての組） |
| --- | --- | --- |
| 10 人・話者 2 人の組の数 | 17 | 45 |
| 話者の切り替え | 結び直しが要る | 再ネゴシエーションだけ |
| 聞き手の回線 | 話者の数だけ受ける | 全員と結ぶが音声は話者からだけ |

P2 は話者の切り替えが単純だが、聞き手どうしの接続は音声を運ばず、ICE と TURN の割り当てを無駄に使う（`relay` では TURN の料金にも効く）。

## Open Questions / 要検証

- coturn の `use-auth-secret` の HMAC の既定のハッシュ関数が版によらず SHA-1 か（要検証: coturn の版ごとの文書）
- TURN の資格情報の期限が切れたとき、確立済みの割り当て（allocation の refresh）が切れるか（要検証: coturn と Cloudflare。切れるなら §5 の取り直しを期限の前に必ず行う）
- iOS の PWA を背面にしたときの WebRTC の音声の継続（要検証: 実機）
