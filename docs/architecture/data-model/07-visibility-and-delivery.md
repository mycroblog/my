## 7. 公開範囲と配送

| 公開範囲 | 配送先（作成時） | `Update` / `Delete` の宛先 |
| --- | --- | --- |
| `public` | 作者のリモートフォロワー + メンション先 + 参加中（`accepted`）のリレーの inbox（ADR-0061） | 作成時の `deliveries` + その時点のフォロワー |
| `unlisted` | 作者のリモートフォロワー + メンション先 | 作成時の `deliveries` + その時点のフォロワー |
| `local` | 作者の **リモートフォロワーだけ**。フォロワーでないメンション先には送らない（ADR-0017、オーナー確認済み） | 作成時の `deliveries` |
| `private` | 作者のリモートフォロワー + メンション先 | 作成時の `deliveries` |
| `direct` | メンション先 | 作成時の `deliveries` |

1. 投稿のトランザクションで、`outbound_activities`（`Create`）と `jobs`（`federation.fanout` ジョブ、`EnqueueTx`）を記録します。
2. `federation.fanout` ジョブが宛先を一定件数ずつ算出し、`deliveries` に挿入します（`ON CONFLICT DO NOTHING`）。挿入できた行ごとに `federation.deliver` ジョブを投入し、カーソルを進めます（[federation.md §7.2](../federation.md#72-展開fan-out)）。
3. `federation.deliver` の結果は `delivery_attempts` に記録し、同期 projector が `instance_health` を更新します。
4. 配送先の算出とオブジェクト IRI への GET は、`canView` で判定します（ADR-0017, ADR-0018、[公開範囲の閲覧判定](../visibility.md)）。
   - `public` / `unlisted` は署名がなくても返します。
   - `local` は「署名者のドメインに作者の `active` なフォロワーがいる」場合だけ返します。`private` はそれに加えて署名者のドメインにメンション先がいる場合も、`direct` はメンション先がいる場合だけ返します。それ以外は 404 にします。
   - 作者に `profile_view` の制限があれば 404 にします。
