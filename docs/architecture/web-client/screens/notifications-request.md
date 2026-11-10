---
start: "/notifications"
persona: "yuki"
paths: ["app/web/src/routes/notifications_.requests_.$accountId.tsx", "app/web/src/features/notifications/components/NotificationRequest.tsx", "app/web/src/features/notifications/hooks/useNotificationRequest.ts", "app/web/src/features/notifications/api/requests.ts"]
unavailable: {"リクエストの通知を読んで受け入れる": "the demo scenario keeps every notification policy at accept, so no notification is held in a request"}
---

## リクエストの通知（`/notifications/requests/{account_id}`）

1 人の差出人の通知のリクエストに保留した通知を読み、受け入れるか閉じるかを決める画面（[ADR-0158](../../../adr/0158-notification-policy.md) §7、§10）。
[通知のリクエスト](notifications-requests.md)の一覧では数しか分からないので、何が保留されたかを確かめてから答えるために開く。

### 1. 使う人と頻度

通知のリクエストを受け取った利用者。知らない相手の通知を受け入れる前に、中身を確かめるときに開く。リクエストのたびに多くて 1 回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| リクエストの通知を読んで受け入れる | 3 |

手順は、通知の画面のリクエストの行、リクエストの「通知を見る」、この画面の「受け入れる」の 3 つ（通知を読むことは数えない）。

### 3. 情報の優先順位とまとまり

画面の `h1` は「保留した通知」で、見出しに戻る操作（`back`）を持つ。区画の `h2` は持たない。

1. 差出人と答え: 差出人（アバター・名前・ハンドル。プロフィールへのリンク）と保留した通知の数、受け入れる・閉じるが何をするかの 1 行、「受け入れる」（`primary`）・「閉じる」。
2. 保留した通知の一覧（`feed`、名前は `h1`）: 通知の画面と同じ行（`NotificationItem`。同じ投稿へのお気に入りとブーストは 1 行にまとめる）を新しい順に。未読の印は付けない（保留した通知は既読の位置と関わらない）。
3. 一覧の終わり: 続きの読み込み

リクエストがない（答え済み・差出人をブロックした・その人のリクエストがない）ときは、ルートのエラーとして「リクエストが見つかりません」と、[通知のリクエスト](notifications-requests.md)へのリンクを出す。

### 4. 主な操作と保存の仕方

- 主な操作は、リクエストに答えること（受け入れる・閉じる）。すぐ送り、確かめない。
- 答えると、[通知のリクエスト](notifications-requests.md)の一覧へ戻る。キャッシュの変わり方は一覧で答えたときと同じ。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 受け入れる | ボタンが送信中になり、終わると[通知のリクエスト](notifications-requests.md)へ遷移し、トースト「{名前} さんの通知を受け入れました」 |
| 閉じる | 受け入れると同じで、トーストは「{名前} さんの通知のリクエストを閉じました」 |
| 答えが失敗する | その場。ボタンの下に理由（`InlineError`）、フォーカスは押したボタンに残る |
| 通知の行の操作（投稿を開く・プロフィールを開く） | 通知の画面と同じ |
| 一覧の読み込みが失敗する | その場。一覧の位置に `RegionError`（何が起きたか・理由・もう一度試す） |

### 6. 入口と出口

- 入口: [通知のリクエスト](notifications-requests.md)の行の「通知を見る」。
- 出口: 戻る・答えた後で[通知のリクエスト](notifications-requests.md)。通知の行から投稿の詳細・プロフィール。
