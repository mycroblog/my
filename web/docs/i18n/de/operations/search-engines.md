<!-- source: search-engines 6816d50f42dd -->
# Suchmaschinen

Wie du jede Suchmaschine betreibst, die die Volltextsuche in Beiträgen nutzen kann, wie groß sie sein muss und wie weit sie Japanisch und andere Sprachen findet, die ohne Leerzeichen geschrieben werden. Für eine Engine, die hier nicht steht, etwa Groonga oder einen mit Sudachi gebauten Index, kann stattdessen [ein Such-Plugin](plugins.md#searching-posts) von dir den Index halten. [Volltextsuche in Beiträgen](search.md) behandelt, was der Index enthält, wie du die Suche einschaltest, den Index neu aufbaust und die Engine wechselst, unabhängig von der Engine.

<!-- source: elasticsearch ed41ee8526d2 -->
## Elasticsearch

`search = elasticsearch` hält den Index in einem Elasticsearch-Cluster der Version 8 oder neuer, selbst verwaltet oder auf Elastic Cloud Hosted. Er wurde gegen Elasticsearch 9.5 und 8.19 geprüft. Der Server besitzt die Felder des Index, auf denen er schreibt, filtert, sortiert und blättert. Die Einstellungen darfst du besitzen: die Analyse, die Shards und die Replikate. Ohne eigene Einstellungen brauchen die eingebauten kein Plugin und finden Japanisch wie [unten beschrieben](#how-the-built-in-analysis-finds-japanese).

<!-- source: setting-it-up 3e02c35ff319 -->
### Einrichten

1. Betreibe Elasticsearch. Ein einzelner Knoten reicht für die meisten Server. Für einen schnellen Start auf demselben Host:

   ```sh
   docker run -d --name elasticsearch -p 127.0.0.1:9200:9200 \
     -e discovery.type=single-node -e ES_JAVA_OPTS="-Xms1g -Xmx1g" \
     -e ELASTIC_PASSWORD=change-me \
     -v esdata:/usr/share/elasticsearch/data \
     elasticsearch:9.5.3
   ```

   Mit eingeschalteter Sicherheit, was der Standard ist, liefert Elasticsearch HTTPS mit einem Zertifikat einer eigenen Zertifizierungsstelle aus. Kopiere diese Zertifizierungsstelle für `search_elasticsearch_ca_file` aus dem Container:

   ```sh
   docker cp elasticsearch:/usr/share/elasticsearch/config/certs/http_ca.crt /etc/my/elasticsearch-ca.crt
   ```

2. Erstelle einen API-Schlüssel, der nur die Indizes des Servers verwalten kann. `manage`, `read` und `write` auf dem Alias und den Indizes dahinter sind alles, was er braucht:

   ```sh
   curl --cacert /etc/my/elasticsearch-ca.crt -u elastic:change-me -X POST https://localhost:9200/_security/api_key \
     -H 'Content-Type: application/json' -d '{
       "name": "my",
       "role_descriptors": {"my": {"indices": [{"names": ["my-statuses*"], "privileges": ["manage", "read", "write"]}]}}
     }'
   ```

   Der Wert `encoded` der Antwort ist der Schlüssel.

3. Setze die Konfiguration des Servers und starte ihn neu:

   ```sh
   MY_SEARCH=elasticsearch
   MY_SEARCH_ELASTICSEARCH_URL=https://localhost:9200
   MY_SEARCH_ELASTICSEARCH_API_KEY=<the encoded value>
   MY_SEARCH_ELASTICSEARCH_CA_FILE=/etc/my/elasticsearch-ca.crt
   ```

4. Führe `my search rebuild` aus, um die vorhandenen Beiträge hinzuzufügen, wie unter [Einschalten](search.md#turning-it-on).

Die Einstellungen:

| Einstellung | Was sie bewirkt |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | Die URL der REST-API. Nutze `https:`, wann immer die Anfragen ein Netzwerk durchqueren, das andere mitnutzen; `http:` wird für ein privates Netzwerk oder einen Cluster ohne Sicherheit akzeptiert. |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | Der API-Schlüssel, die bessere Wahl. |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username), [`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | Ein Benutzer und Passwort für die Basic-Authentifizierung statt eines Schlüssels. |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | Der Name des Alias, standardmäßig `my-statuses`. Gib jedem Server, der sich einen Cluster teilt, einen eigenen Namen und gewähre seinem Schlüssel diesen Namen gefolgt von `*`. |
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | Eine JSON-Datei mit deinen eigenen Einstellungen des Index, wie unter [Die Analyse von Elasticsearch anpassen](#tuning-the-analysis-of-elasticsearch). |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | Die Zertifizierungsstelle eines HTTPS-Clusters, dessen Zertifikat das System nicht vertraut. |

Die Zugangsdaten werden nur aus der Umgebung gelesen, als Secrets, und erscheinen nie in den Logs oder Fehlern des Servers. Ein Fehler des Clusters erreicht die Logs als Pfad der Anfrage, ihr Status und der Typ des Fehlers, etwa `index_not_found_exception`, nie mit dem Text eines Beitrags oder einer Suchanfrage. Nur wenn der Cluster sich weigert, den Index anzulegen, zitiert der Fehler seinen Grund, der nichts außer den Einstellungen und den Feldern nennt.

<!-- source: the-index-and-its-upgrades 319d3f3a4bd2 -->
### Der Index und seine Upgrades

Der Server legt den Index selbst an und pflegt ihn. Lege ihn nicht von Hand an und lass nichts anderes darauf schreiben.

- Beim Start sucht der Server den Alias. Gibt es keinen, legt er einen Index an, benannt nach dem Alias, der Version seiner Felder und einem Hash seiner Einstellungen, etwa `my-statuses-v1-1f0c2e9a7b3d4c5e`, und richtet den Alias darauf. Die Version und der Hash werden im `_meta` des Index gehalten.
- Der Server kann nicht starten, solange der Cluster nicht erreichbar ist, seinen Schlüssel ablehnt oder die Einstellungen ablehnt. `my config check` prüft die Konfiguration und dass die Einstellungsdatei ein einzelnes JSON-Objekt ist, nicht was Elasticsearch davon hält, also prüfe die Verbindung und die Einstellungen, indem du den Server startest.
- Ein Index mit dem Namen des Alias, den der Server nicht angelegt hat, etwa einer, den Elasticsearch selbst erzeugt hat, stoppt den Start mit einem Fehler, der ihn nennt. Lösche diesen Index oder wähle ein anderes `search_elasticsearch_index`.
- Wenn sich die Einstellungen von denen unterscheiden, mit denen der Index angelegt wurde, weil du die Einstellungsdatei bearbeitet hast oder eine neue Version von `my` die eingebauten geändert hat, legt der Server einen neuen leeren Index mit den neuen Einstellungen an, richtet den Alias darauf, löscht den alten und protokolliert eine Warnung, die `my search rebuild` verlangt. Führe es aus. Bis der Neuaufbau endet, findet die Suche nur die seitdem geschriebenen Beiträge. Wenn Elasticsearch die neuen Einstellungen ablehnt, startet der Server nicht, und der alte Index bleibt.
- Wenn eine neue Version von `my` die Felder ändert, führt der Server beim Start ein Upgrade des Index durch. Eine Änderung, die nur Felder hinzufügt, erfolgt an Ort und Stelle, und die Suche funktioniert weiter; die Release Notes sagen, wann die Beiträge, die schon im Index sind, `my search rebuild --keep` brauchen, um die neuen Felder zu bekommen. Jede andere Änderung der Felder erfordert einen neuen leeren Index und einen Neuaufbau, wie oben. Die Release Notes sagen, wann eine Version das tut.
- Ein Index, den eine neuere Version von `my` angelegt hat, verhindert, dass eine ältere startet. Um über eine solche Änderung zurückzugehen, lösche den Index des Alias in Elasticsearch (finde ihn mit `GET _alias/my-statuses`), starte die ältere Version, die ihren eigenen anlegt, und führe `my search rebuild` aus.
- `my search clear` richtet den Alias in einem Schritt auf einen neuen leeren Index und löscht den alten, was schneller ist, als jedes Dokument zu löschen, und den Speicherplatz zurückgibt.

<!-- source: sizing 2b1d9b604953 -->
### Dimensionierung

Der Index hält nur, was eine Suche braucht, und keine Kopie des Textes (`_source` ist aus). Mit den eingebauten Einstellungen belegen kurze Beiträge etwa 80 bis 150 Byte Festplatte pro Stück, sobald Elasticsearch seine Segmente zusammengeführt hat, und längere Beiträge mit größerem Wortschatz mehr, also plane mit 150 bis 300 Byte pro Beitrag: ein paar hundert Megabyte für eine Million Beiträge. Prüfe die tatsächliche Größe mit `GET _cat/indices/my-statuses*?v`.

Die eingebauten Einstellungen geben dem Index einen Primär-Shard, der zig Millionen Beiträge hält, und ein Replikat, sobald der Cluster einen zweiten Knoten hat (`auto_expand_replicas: 0-1`), sodass ein einzelner Knoten grün bleibt. Setze `number_of_shards` und `number_of_replicas` in einer Einstellungsdatei, um sie zu ändern.

Ein Knoten mit 1 GB Java-Heap (`ES_JAVA_OPTS=-Xms1g -Xmx1g`) und insgesamt etwa 2 GB Speicher bedient einen kleinen oder mittleren Server. Gib Elasticsearch die Hälfte des Speichers der Maschine als Heap und überlasse den Rest dem Dateicache, und gib mehr, wenn Suchen langsamer werden. Replikate brauchen mehr Knoten und für jedes Replikat noch einmal denselben Festplattenplatz.

<!-- source: how-a-query-uses-the-analysis 20b451dd32c6 -->
### Wie eine Suchanfrage die Analyse nutzt

Jeder Beitrag wird unabhängig von seiner Sprache in ein Textfeld indiziert, und dieselbe Analyse gilt für alle. Die Sprache eines Beitrags wählt keinen anderen Analyzer.

- Der Text eines Beitrags wird mit dem Analyzer namens `my_text` analysiert.
- Eine Suchanfrage wird an Leerzeichen, auch am ideographischen Leerzeichen, in Wörter geteilt. Jedes Wort wird mit `my_text_search` analysiert, wenn die Einstellungen ihn definieren, und sonst mit `my_text`, und muss im Beitrag als Phrase vorkommen: seine Tokens in derselben Reihenfolge und direkt nebeneinander. Jedes Wort muss passen. Nichts in einer Suchanfrage wird als Abfragesprache von Elasticsearch gelesen.
- Die Ergebnisse sind nach neuesten zuerst geordnet, nicht nach Relevanz, daher machen Scoring-Optionen keinen Unterschied.

Ein Analyzer entscheidet also, was ein Wort ist. Ein Analyzer, der aus jedem Zeichen ein Token macht, findet jede Zeichenfolge. Einer, der Wörterbuchwörter macht, findet ganze Wörter und ihre anderen Formen.

<!-- source: how-the-built-in-analysis-finds-japanese e260562ab16a -->
### Wie die eingebaute Analyse Japanisch findet

Das eingebaute `my_text` ist der Standard-Tokenizer von Elasticsearch mit den Filtern `cjk_width` und `lowercase`, alle eingebaut. Der Tokenizer macht aus jedem Kanji und jedem Hiragana, aus jeder Folge von Katakana und aus jedem Wort der Sprachen, die Leerzeichen verwenden, ein Token.

- Ein Wort aus Kanji oder Hiragana wird gefunden, wo immer seine Zeichen zusammenstehen, auch innerhalb eines längeren Wortes: `検索` und `本` finden `全文検索の本文です`, und `東京 天気` findet `東京都の天気は晴れ`. Zeichen, die nur getrennt vorkommen, passen nicht.
- Eine Folge von Katakana wird als Ganzes abgeglichen: `カタカナ` findet `カタカナとABC`, aber `カタ` findet es nicht, und `ツリー` findet `スカイツリー` nicht.
- Vollbreite lateinische Buchstaben und halbbreite Katakana passen auf ihre üblichen Formen: `abc` findet `ＡＢＣ`, und `カタカナ` findet `ｶﾀｶﾅ`.
- Hiragana findet nicht das Kanji mit derselben Lesung: `ぜんぶん` findet `全文` nicht. Andere Formen eines Wortes werden nicht gefunden: `走る` findet `走った` nicht.
- Eine Suchanfrage mit einem sehr häufigen Zeichen wie `の` findet sehr viele Beiträge.

Chinesisch und Koreanisch werden auf dieselbe Weise über ihre Zeichen gefunden. Wörter in den Sprachen, die mit Leerzeichen geschrieben werden, werden ganz gefunden, ohne Stemming: `run` findet `running` nicht.

<!-- source: tuning-the-analysis-of-elasticsearch fb5ddcfd414e -->
### Die Analyse von Elasticsearch anpassen

Lege die Einstellungen des Index in eine JSON-Datei und nenne sie in `search_elasticsearch_index_settings_file`. Der Server nutzt die ganze Datei als `settings` jedes Index, den er anlegt, anstelle der eingebauten, daher muss sie alles enthalten, was der Index braucht:

- Den Analyzer `my_text`, den das Textfeld nutzt. Ohne ihn weigert sich Elasticsearch, den Index anzulegen, und der Server startet nicht.
- Optional den Analyzer `my_text_search`, der dann die Wörter der Suchanfragen analysiert.
- Jede andere Index-Einstellung, etwa `number_of_shards` und `number_of_replicas`. Was die Datei weglässt, nimmt die Standardwerte des Clusters.
- Nur Analyse, die Elasticsearch hat: Ein Plugin, das die Datei nennt, muss auf jedem Knoten installiert sein.

Der Server liest die Datei beim Start. Wird sie bearbeitet, bekommt der nächste Start einen neuen leeren Index, wie unter [Der Index und seine Upgrades](#the-index-and-its-upgrades), also führe nach jeder Änderung `my search rebuild` aus. Probiere eine Änderung zuerst auf einem anderen Cluster oder mit einem anderen `search_elasticsearch_index` aus.

Wörterbuchwörter mit dem Plugin `analysis-kuromoji`, das du auf jedem Knoten installierst (`bin/elasticsearch-plugin install analysis-kuromoji`) oder das Elastic Cloud Hosted als Erweiterung anbietet:

```json
{
  "number_of_shards": 1,
  "number_of_replicas": 0,
  "analysis": {
    "analyzer": {
      "my_text": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["cjk_width", "lowercase", "kuromoji_baseform"]
      },
      "my_text_search": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["cjk_width", "lowercase", "kuromoji_baseform"]
      }
    }
  }
}
```

Jede Folge von einem oder zwei Zeichen, nur mit der in Elasticsearch eingebauten Analyse:

```json
{
  "number_of_shards": 1,
  "number_of_replicas": 0,
  "max_ngram_diff": 1,
  "analysis": {
    "tokenizer": {
      "my_grams": { "type": "ngram", "min_gram": 1, "max_gram": 2, "token_chars": ["letter", "digit"] }
    },
    "analyzer": {
      "my_text": { "type": "custom", "tokenizer": "my_grams", "filter": ["cjk_width", "lowercase"] }
    }
  }
}
```

Wie sie sich vergleichen, gemessen an 20.000 Beiträgen, zur Hälfte Englisch und zur Hälfte Japanisch, auf Elasticsearch 9.5:

| | Eingebaut | Kuromoji | Ein und zwei Zeichen |
| --- | --- | --- | --- |
| Plugin | Keines | `analysis-kuromoji` | Keines |
| Größe des Index | 1,5 MB | 1,4 MB | 3,7 MB |
| Teil eines Kanji-Wortes: `京都` in `東京都` | Gefunden | Nicht gefunden | Gefunden |
| Teil eines Katakana-Wortes: `ツリー` in `スカイツリー` | Nicht gefunden | Gefunden | Gefunden |
| Teil eines Katakana-Wortes: `コンピュータ` in `コンピューター` | Nicht gefunden | Nicht gefunden | Gefunden |
| Andere Form eines Wortes: `走る` findet `走った` | Nicht gefunden | Gefunden | Nicht gefunden |
| Teil eines englischen Wortes: `run` in `running` | Nicht gefunden | Nicht gefunden | Gefunden |

Kuromoji teilt Japanisch in die Wörter seines Wörterbuchs und führt jedes auf seine Grundform zurück, sodass es ein Wort in jeder seiner Formen findet, und ein Wort, das das Wörterbuch in einem Kompositum kennt. Es findet keinen Teil eines Wortes, das das Wörterbuch ganz hält, und wie es ein neues Wort oder einen Namen teilt, hängt vom Wörterbuch ab.

Folgen von einem und zwei Zeichen finden jeden Teil jedes Wortes in jeder Sprache, auch Teile, die keine Wörter sind: `run` findet auch `brunch`. Der Index ist etwa zweieinhalbmal so groß, und eine Suche liest mehr davon.

Suchen dauerten mit jedem von ihnen auf diesem kleinen Index ein paar Millisekunden, und die Unterschiede wachsen mit der Größe des Index.

<!-- source: opensearch fe2924a87307 -->
## OpenSearch

`search = opensearch` hält den Index in einem OpenSearch-Cluster, den du betreibst, oder in einer Domain von Amazon OpenSearch Service. Der Server spricht mit der REST-API des Clusters. Er besitzt die Felder des Index und die Anfragen, die er sendet, und den Rest darfst du besitzen: die Analyzer, die Shards, die Replikate und jede andere Index-Einstellung. Ohne eigene Einstellungen nutzt er in OpenSearch eingebaute Analyzer und braucht kein Plugin.

<!-- source: setting-it-up-1 11097d66bccc -->
### Einrichten

1. Betreibe OpenSearch 2 oder 3. Ein Knoten reicht zum Anfangen; der Server erreicht immer nur die Adresse, die du ihm gibst.
2. Setze [`search`](../reference/configuration.md#search) auf `opensearch` und [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) auf die URL der REST-API, etwa `https://search.internal.example.org:9200`. Ein Pfad bleibt erhalten, für einen Cluster hinter einem Reverse-Proxy.
3. Gib dem Server einen Zugang:
   - Mit dem Security-Plugin einen Benutzer mit [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) und [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password), gesendet mit HTTP-Basic-Authentifizierung. Nutze `https`, wenn der Cluster nicht auf demselben Host oder in einem privaten Netzwerk liegt, dem du vertraust. Der Benutzer muss die Indizes, deren Namen mit [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index) beginnen, anlegen, ihr Mapping lesen und sie löschen, diesen Alias verwalten sowie ihre Dokumente lesen, schreiben und löschen können.
   - Auf Amazon OpenSearch Service mit einer IAM-Zugriffsrichtlinie [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) auf `es` gesetzt, was jede Anfrage mit AWS Signature Version 4 signiert. Die Zugangsdaten kommen aus der Standardkette des AWS SDK (die Ausführungsrolle, die Umgebung, die gemeinsamen Dateien), und die Region aus [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) oder `AWS_REGION`. Die Rolle braucht `es:ESHttpGet`, `es:ESHttpPost`, `es:ESHttpPut` und `es:ESHttpDelete` auf der Domain. Eine Domain mit feingranularer Zugriffskontrolle und einem internen Master-Benutzer nimmt stattdessen Benutzer und Passwort.
   - Amazon OpenSearch Serverless wird nicht unterstützt: Es hat kein Delete-by-Query, das das Entfernen der Beiträge einer Autorin oder eines Autors oder einer Sichtbarkeit braucht.
4. Passe optional [die Analyse an](#tuning-the-analysis-of-opensearch), mit einer Einstellungsdatei.
5. Starte den Server. Er legt den Index beim Start an und startet nicht, wenn er den Cluster nicht erreichen oder den Index nicht anlegen kann.
6. Führe `my search rebuild` aus, um die bereits vorhandenen Beiträge zu indizieren ([den Index neu aufbauen](search.md#rebuilding-the-index)).

Die URL, der Benutzer, die AWS-Zugangsdaten und der Inhalt der Einstellungsdatei erscheinen nie in den Logs oder Fehlern des Servers. Ein Fehler der Engine erreicht das Log nur als Operation, HTTP-Status und Typ des Fehlers der Engine, da die eigene Meldung der Engine einen Beitrag oder eine Suchanfrage zitieren kann. Die einzige Ausnahme ist eine abgelehnte Anlage des Index, deren Meldung die fehlerhafte Einstellung nennt.

<!-- source: the-index-and-its-upgrades-1 5529569168b1 -->
### Der Index und seine Upgrades

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), standardmäßig `my-statuses`, benennt einen Alias. Die Dokumente liegen in einem Index, den der Server anlegt und auf den er den Alias richtet, benannt nach dem Alias mit der Version des Mappings des Servers und einem Hash der Index-Einstellungen, etwa `my-statuses-v1-3f2a9c41b07e`. Das Mapping hält dieselben beiden in seinem `_meta` fest (`my_mapping_version` und `my_settings_hash`). Jedes Schreiben läuft über den Alias und schlägt fehl, solange er fehlt, sodass OpenSearch nie einen eigenen Index mit einem erratenen Mapping anlegt. Lege keinen Index dieses Namens selbst an: Der Server startet nicht, wenn der Name ein Index statt eines Alias ist oder wenn der Alias auf einen Index zeigt, den er nicht angelegt hat.

Beim Start vergleicht der Server den Index hinter dem Alias mit seiner eigenen Mapping-Version und seinen Einstellungen. Unterscheidet sich eines von beiden (ein Release, dessen Mapping sich geändert hat, was seine Release Notes sagen, oder eine bearbeitete Einstellungsdatei), legt er einen neuen Index an und verschiebt den Alias darauf. Der neue Index beginnt leer, also:

1. Führe `my search rebuild` aus. Bis es endet, funktioniert die Suche, findet aber nur, was seit der Änderung gepostet oder neu aufgebaut wurde.
2. Lösche den alten Index (`DELETE /my-statuses-v1-…`), wenn der Neuaufbau beendet ist. Der Server lässt ihn bestehen und protokolliert eine Warnung, die ihn nennt.

Starte jeden Prozess des Servers mit derselben Einstellungsdatei: Prozesse mit unterschiedlichen Dateien verschieben den Alias hin und her. Ein Server eines älteren Releases, der gegen einen Index eines neueren Mappings startet, nutzt diesen Index, wie er ist.

`my search clear` und `my search rebuild` ohne `--keep`, das damit beginnt, legen einen neuen leeren Index mit demselben Mapping und denselben Einstellungen an, verschieben den Alias darauf und löschen den alten in einer Anfrage.

<!-- source: sizing-1 4ad4bbcd9a2b -->
### Dimensionierung

- **Speicher**: Der Knoten braucht einen JVM-Heap von mindestens 512 MB (`-Xms512m -Xmx512m`) und noch einmal so viel für das Betriebssystem; gib ihm 1 GB für einen kleinen Server und mehr, wenn der Index wächst. Der Heap hält man am besten bei der Hälfte des Speichers des Knotens.
- **Festplatte**: Der Index belegt mit den eingebauten Einstellungen etwa die Größe des Textes der indizierten Beiträge, mehr bei Sprachen, die ohne Leerzeichen geschrieben werden, deren Text als einzelne Zeichen und Paare indiziert wird, und mit N-Grammen noch mehr. Halte mindestens 15 % der Festplatte frei: Standardmäßig platziert OpenSearch ab 85 % keine Shards mehr auf einem Knoten und blockiert ab 95 % Schreibzugriffe auf dessen Indizes.
- **Shards und Replikate**: Ohne Einstellungsdatei hat der Index die Standardwerte des Clusters, einen Shard und ein Replikat, sofern du sie nicht geändert hast. Ein Shard bedient Millionen von Beiträgen. Ein Replikat lässt einen Einzelknoten-Cluster gelb; es funktioniert, und `"index": {"number_of_replicas": 0}` in einer Einstellungsdatei macht ihn grün. Behalte auf einem Cluster aus mehreren Knoten ein Replikat. Die Zahl der Replikate lässt sich auch am laufenden Index ändern (`PUT /my-statuses/_settings`), was keinen Neuaufbau braucht; der Server nutzt den Index weiter, solange sich die Einstellungsdatei nicht ändert.
- **Last**: Jede Suche ist eine Abfrage des Index, neueste zuerst nach Beitrags-ID. Schreibzugriffe sind eine Anfrage pro Beitrag, und ein Neuaufbau schreibt nicht schneller als [seine Rate](search.md#rebuilding-the-index).

<!-- source: japanese-and-other-languages a48cff99706b -->
### Japanisch und andere Sprachen

Mit den eingebauten Einstellungen wird jeder Beitrag unabhängig von seiner Sprache gleich analysiert: Der Text wird an Leerzeichen und Satzzeichen in Wörter geteilt, in Kleinbuchstaben umgewandelt, und vollbreite lateinische Buchstaben und halbbreite Katakana werden auf ihre üblichen Formen gefaltet. Chinesischer, japanischer und koreanischer Text, der keine Leerzeichen hat, wird in Zeichen geteilt, und jedes Paar benachbarter Zeichen wird zusätzlich zu jedem Zeichen indiziert (der in OpenSearch eingebaute Filter `cjk_bigram`).

- Eine Suchanfrage findet die Beiträge, die jedes ihrer Wörter enthalten. Im Japanischen findet ein Wort mit zwei oder mehr Zeichen die Beiträge, die jedes Paar seiner Zeichen enthalten: `東京` findet `東京都の天気`, und `全文検索` findet `全文検索の本文です`. Ein einzelnes Zeichen wie `本` findet jeden Beitrag mit diesem Zeichen.
- Die Paare wissen nicht, wo Wörter enden, daher kann eine Suchanfrage einen Beitrag finden, in dem ihre Zeichen nur zufällig nebeneinander stehen: `京都` findet auch `東京都`. Eine Suchanfrage in Katakana, die in halbbreiten oder vollbreiten Buchstaben geschrieben ist, findet beide.
- Lesungen werden nicht abgeglichen: `ぜんぶん` findet `全文` nicht, und gebeugte Formen werden nicht auf ihre Wörterbuchformen abgebildet, weder im Japanischen noch im Englischen (`running` findet `run` nicht).

Um Japanisch nach Wörtern zu finden, nutze einen morphologischen Analyzer wie Kuromoji in einer [Einstellungsdatei](#tuning-the-analysis-of-opensearch).

<!-- source: tuning-the-analysis-of-opensearch 1289083a07ec -->
### Die Analyse von OpenSearch anpassen

[`search_opensearch_index_settings_file`](../reference/configuration.md#search_opensearch_index_settings_file) nennt eine JSON-Datei, deren Objekt zu den `settings` jedes Index wird, den der Server anlegt: `analysis`, `index.number_of_shards`, `index.number_of_replicas` und jede andere Index-Einstellung. Sie enthält nur Einstellungen; der Server besitzt das Mapping und lehnt eine Datei mit `mappings` ab.

Die Datei muss zwei Analyzer definieren, die das Mapping nennt:

| Analyzer | Analysiert |
| --- | --- |
| `my_text` | Den Text jedes Beitrags: seinen Text, die Inhaltswarnung und die Alternativtexte seiner Medien |
| `my_text_search` | Den Text jeder Suchanfrage |

Sie dürfen dieselbe Definition haben. Jede Suchanfrage ist eine `match`-Abfrage des Feldes `text` mit `"operator": "and"`: Der Text der Anfrage läuft durch `my_text_search`, und ein Beitrag wird gefunden, wenn sein Text jeden herauskommenden Term enthält, neueste zuerst nach Beitrags-ID, ohne Scoring. Eine Suchanfrage, deren Analyzer keinen Term übrig lässt, findet nichts. Phrasen-, Fuzzy- und Präfix-Abgleich werden nicht verwendet, sodass ein Analyzer allein entscheidet, was ein Treffer ist. Die Sprache eines Beitrags wird gespeichert, wählt aber keinen Analyzer: Jeder Beitrag läuft durch `my_text`.

Wenn der Server mit einer Datei startet, deren Einstellungen sich von denen des Index unterscheiden, auch nur um ein Replikat, legt er einen neuen leeren Index an, und der Index braucht einen Neuaufbau ([Der Index und seine Upgrades](#the-index-and-its-upgrades)). Formatierung und Reihenfolge der Schlüssel zählen nicht. Eine Datei, die OpenSearch ablehnt, etwa eine ohne die beiden Analyzer oder mit einem Plugin, das der Cluster nicht hat, stoppt den Start mit dem Grund von OpenSearch.

Kuromoji teilt Japanisch mit einem Wörterbuch in Wörter und gleicht gebeugte Formen anhand ihrer Wörterbuchformen ab. Amazon OpenSearch Service hat das Plugin auf jeder Domain; ein Cluster, den du betreibst, braucht es installiert (`opensearch-plugin install analysis-kuromoji`, zum Beispiel in einem Dockerfile `FROM opensearchproject/opensearch`). Eine Datei mit Kuromoji:

```json
{
  "index": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "analysis": {
    "analyzer": {
      "my_text": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["kuromoji_baseform", "kuromoji_part_of_speech", "cjk_width", "kuromoji_stemmer", "lowercase"]
      },
      "my_text_search": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["kuromoji_baseform", "kuromoji_part_of_speech", "cjk_width", "kuromoji_stemmer", "lowercase"]
      }
    }
  }
}
```

Ein Wort, das das Wörterbuch anders teilt als die Suchanfrage, wird nicht gefunden, daher kann Kuromoji verpassen, was die eingebauten Paare finden, etwa einen Teil eines zusammengesetzten Substantivs oder ein neues Wort. Ein N-Gramm-Tokenizer, in OpenSearch eingebaut, findet jede Zeichenfolge, zum Preis eines größeren Index und von Treffern innerhalb längerer Wörter; mit `"min_gram": 2` und `"max_gram": 3` (und `"index": {"max_ngram_diff": 1}`) findet eine Suchanfrage mit einem einzelnen Zeichen nichts.

<!-- source: meilisearch 51ac396ba9d0 -->
## Meilisearch

`search = meilisearch` hält den Index auf einem [Meilisearch](https://www.meilisearch.com/docs)-Server, selbst gehostet oder Meilisearch Cloud. Es ist ein einzelner Prozess ohne Cluster, der zu betreiben wäre, was zu einem kleinen Server passt, und das offizielle Image segmentiert Japanisch, Chinesisch und Koreanisch ohne Plugin. Der Server wurde gegen Meilisearch 1.56 geprüft.

<!-- source: running-meilisearch 4f7d5e17c255 -->
### Meilisearch betreiben

Betreibe das offizielle Image mit einem Master Key von mindestens 16 Byte, in einem Netzwerk, das nur `my` erreichen kann, und halte sein Datenverzeichnis auf einem persistenten Volume:

```sh
docker run -d --name meilisearch --restart unless-stopped \
  -p 127.0.0.1:7700:7700 \
  -e MEILI_ENV=production -e MEILI_NO_ANALYTICS=true \
  -e MEILI_MASTER_KEY="$(openssl rand -base64 32)" \
  -v meili_data:/meili_data \
  getmeili/meilisearch:v1.56.1
```

`MEILI_ENV=production` weigert sich, ohne Master Key zu starten, und schaltet die eingebaute Such-Vorschau aus. Meilisearch spricht einfaches HTTP: Setze es hinter TLS oder halte es in einem privaten Netzwerk, wenn `my` es über etwas anderes als eine lokale oder private Verbindung erreicht.

<!-- source: settings 74547d71e962 -->
### Einstellungen

```toml
search = "meilisearch"
search_meilisearch_url = "http://meilisearch:7700"
# search_meilisearch_index = "my_statuses"
# search_meilisearch_settings_file = "/etc/my/meilisearch-settings.json"
```

Setze den Schlüssel über die Umgebung (`MY_SEARCH_MEILISEARCH_API_KEY` oder `MY_SEARCH_MEILISEARCH_API_KEY_FILE`), nie in der Konfigurationsdatei. Es darf der Master Key sein, aber ein Schlüssel, der nur kann, was `my` braucht, ist sicherer. Erstelle einen mit dem Master Key:

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.get", "documents.delete", "indexes.create", "indexes.get",
      "indexes.delete", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

Nutze den `key` der Antwort als `search_meilisearch_api_key`. Der Schlüssel wird nur im Header `Authorization` gesendet: Die Logs und Fehler des Servers zeigen den Fehlercode von Meilisearch (etwa `invalid_api_key`) und nie den Schlüssel, einen Beitrag oder eine Suchanfrage. Alle [Einstellungen](../reference/configuration.md#search_meilisearch_url) stehen in der Konfigurationsreferenz.

Mehrere Server können sich einen Meilisearch-Server teilen, jeder mit einem anderen `search_meilisearch_index` und einem Schlüssel, der auf seinen eigenen Index beschränkt ist.

<!-- source: the-index-and-its-settings 2657dc55a7bc -->
### Der Index und seine Einstellungen

Der Server legt den Index an, wenn er nicht existiert. Beim ersten Mal, dass er den Index nach einem Start nutzt, vergleicht er die Einstellungen, die er senden würde, mit denen, die er zuletzt angewendet hat, die er als Hash in einem Dokument des Index festhält, das keine Suche findet, und sendet sie nur, wenn sie sich unterscheiden: Ein Neustart ohne Änderung sendet nichts. Die Einstellungen sind:

- **Was der Server selbst setzt:** `searchableAttributes` (die Textfelder der Beiträge), `displayedAttributes` (alle), `filterableAttributes` (die Beitrags-ID, die Autorin oder der Autor und die Sichtbarkeit) und `sortableAttributes` (die Beitrags-ID). Die Suche hängt von ihnen ab, und eine Einstellungsdatei, die eines davon setzt, wird abgelehnt.
- **Was du anpassen kannst:** alles andere, in der [Einstellungsdatei](#tuning-meilisearch). Ohne eine, oder für das, was die Datei weglässt, nutzt der Server seine Standardwerte: `rankingRules` aus `sort`, `words`, `typo`, `proximity`, `attribute` und `exactness`; keine Tippfehlertoleranz; die Sprachen der japanischen, chinesischen und koreanischen Felder in `localizedAttributes`; und die eigenen Standardwerte von Meilisearch für den Rest (`stopWords`, `synonyms`, `dictionary`, `separatorTokens`, `nonSeparatorTokens`, `proximityPrecision`, `pagination`, `faceting`, `distinctAttribute`, `searchCutoffMs`, `prefixSearch`). Einstellungen, die du von Hand an der Engine änderst, bleiben nur, bis der Server das nächste Mal seine Einstellungen sendet.

**Eine Änderung der Einstellungen braucht kein `my search rebuild`.** Meilisearch indiziert die Dokumente, die es hält, selbst im Hintergrund neu, und die Beiträge bleiben im Index. Es braucht nur Zeit, etwa so lange, wie das Indizieren gedauert hat. Der Start des Servers wartet nicht darauf: Der Server sendet die Einstellungen und macht weiter. In der Zwischenzeit antworten Suchen mit den vorherigen Einstellungen, und die Schreibzugriffe des Index warten hinter der Neuindizierung, da Meilisearch die Aufgaben eines Index der Reihe nach ausführt; ein Job, dessen Schreibzugriff nicht innerhalb seiner 10 Sekunden fertig wird, wird später wiederholt, sodass Beiträge, die während der Neuindizierung geschrieben wurden, danach den Index erreichen. `my search clear` wartet auf die Einstellungen des neuen, leeren Index, die keine Zeit brauchen.

Ein Neuaufbau ist nur nötig, wenn der Index leer startet:

- Du änderst `search_meilisearch_index`.
- Du startest einen neuen, leeren Meilisearch-Server, zum Beispiel statt den alten zu upgraden.
- Die Release Notes einer Version von `my` sagen, dass sie die Dokumente anders schreibt.

Schreibzugriffe sind Aufgaben, die Meilisearch der Reihe nach ausführt. Der Server wartet auf die Aufgabe jedes Schreibzugriffs, und eine Aufgabe, die fehlschlägt oder nicht innerhalb der Zeit des Jobs endet, lässt den Job wiederholen, sodass ein Meilisearch, das ausgefallen oder ausgelastet ist, den Index verzögert, ohne eine Änderung zu verlieren. Eine Einstellungsaufgabe, die fehlschlägt, zum Beispiel wegen eines Wertes der Einstellungsdatei, den Meilisearch ablehnt, lässt die Schreibzugriffe fehlschlagen, bis sie gelingt; `GET /tasks?types=settingsUpdate` an der Engine zeigt ihren Fehler. `my search clear` löscht den Index und legt ihn mit den Einstellungen neu an.

Um Meilisearch selbst zu upgraden, folge seiner Upgrade-Anleitung (ein Dump oder sein Dumpless-Upgrade) oder starte die neue Version leer und führe `my search rebuild` aus.

<!-- source: what-a-search-sends 575b89133606 -->
### Was eine Suche sendet

Die Abfrage einer Suche ist fest, was auch immer die Einstellungsdatei sagt: `matchingStrategy: all`, damit ein Beitrag jedes Wort haben muss; `sort: status_id:desc`, neueste zuerst; ein `filter` auf die Beitrags-ID für die nächste Seite; ein `limit` von Seite plus eins; `attributesToRetrieve` der Beitrags-ID; und `showMatchesPosition`, was eine Suchanfrage ohne Wort erkennt. Die doppelten Anführungszeichen einer Phrase und das Minuszeichen vor einem Wort, die Meilisearch als Operatoren läse, werden verworfen, sodass eine Suchanfrage immer aus einfachen Wörtern besteht.

<!-- source: tuning-meilisearch df827448e06c -->
### Meilisearch anpassen

Passe die Engine mit einer JSON-Datei mit [Meilisearch-Index-Einstellungen](https://www.meilisearch.com/docs/reference/api/settings) an, die `search_meilisearch_settings_file` nennt. Der Server prüft sie beim Start, wenn `search = meilisearch`: Sie muss existieren und ein JSON-Objekt sein, darf die vier Einstellungen, die der Server selbst setzt, nicht setzen, und ihre `rankingRules`, falls sie welche hat, müssen mit `sort` beginnen, was die Ergebnisse neueste zuerst hält. Meilisearch prüft die Werte, wenn der Server sie sendet. Der Server protokolliert nie den Inhalt der Datei.

Ein Beispiel, das die Sprachen auf Japanisch einengt, `・` als Trennzeichen behandelt, die Nähe billiger zu indizieren macht und die Tippfehlertoleranz für lange Wörter einschaltet:

```json
{
  "localizedAttributes": [
    {"attributePatterns": ["text_ja"], "locales": ["jpn"]}
  ],
  "separatorTokens": ["・"],
  "proximityPrecision": "byAttribute",
  "typoTolerance": {"enabled": true, "minWordSizeForTypos": {"oneTypo": 8, "twoTypos": 12}},
  "synonyms": {"ご飯": ["ごはん"], "ごはん": ["ご飯"]}
}
```

Was eine Änderung kostet:

- Jede Änderung der Datei lässt Meilisearch beim nächsten Start jedes Dokument neu indizieren, wie oben: Es gehen keine Daten verloren, und nichts braucht einen Neuaufbau, aber Schreibzugriffe warten, bis es endet. Ändere sie zu einer ruhigen Zeit und beobachte die Aufgabe mit `GET /tasks?types=settingsUpdate`.
- `localizedAttributes` und die Tokens entscheiden, wie Text in Wörter geteilt wird: Eine engere Liste von Locales kann einen Beitrag in einer anderen Sprache als die falsche segmentieren (siehe unten), also behalte die japanischen, chinesischen und koreanischen Felder, außer der Server hat keine solchen Beiträge.
- Tippfehlertoleranz und Synonyme lassen eine Suche mehr als die exakten Wörter finden. Die Ranking-Regeln nach `sort` ändern nichts, da keine zwei Beiträge eine ID teilen; sie wegzulassen spart Meilisearch nur ein wenig Arbeit.

<!-- source: sizing-2 1d581fac5316 -->
### Dimensionierung

Meilisearch hält den Index in Memory-Mapped-Dateien auf der Festplatte: Es läuft mit weniger Speicher als der Index und wird schneller, je mehr vom Index in den Speicher passt.

- **Speicher.** 1 GB ist ein vernünftiger Anfang für einen kleinen Server; gib mehr, wenn der Index wächst, idealerweise so viel wie die Größe des Index auf der Festplatte. Die Indizierung nutzt standardmäßig bis zu zwei Drittel des Speichers der Maschine; begrenze sie auf einer Maschine, die mit anderen Diensten geteilt wird, mit `MEILI_MAX_INDEXING_MEMORY` (etwa `512Mb`).
- **Festplatte.** Der Index ist mehrfach größer als der Text, den er hält, weil er die Wortlisten jedes Feldes behält. Miss ihn nach dem ersten Neuaufbau mit `GET /stats` (`databaseSize`) und halte weit mehr freien Platz: Neuindizierung und Upgrades brauchen Platz zum Arbeiten.
- **CPU.** Ein oder zwei Kerne reichen für einen kleinen Server. Ein Neuaufbau ist die schwerste Last; verlangsame ihn mit `my search rebuild --rate`.

<!-- source: japanese-and-other-languages-1 ad5184139aeb -->
### Japanisch und andere Sprachen

Der Server legt den Text eines Beitrags in das Feld seiner Sprache: Japanisch (`ja`, einschließlich Tags wie `ja-JP`), Chinesisch (`zh`), Koreanisch (`ko`) oder ein Feld für jede andere Sprache und für keine Sprache. Er teilt Meilisearch die Sprache der ersten drei mit, und Meilisearch segmentiert jede mit einem Wörterbuch in Wörter:

- **Japanisch** wird von Lindera mit dem UniDic-Wörterbuch segmentiert, ob der Text Kana hat oder nur aus Kanji besteht. `検索`, `全文検索` und `全文　検索` finden alle `全文検索の本文です`, und `東京` findet `東京都庁舎見学`.
- **Chinesisch** wird von jieba segmentiert und **Koreanisch** von Lindera mit dem ko-dic-Wörterbuch.
- **Andere Sprachen** werden an Leerzeichen und Satzzeichen geteilt; Meilisearch erkennt Schrift und Sprache des Textes selbst.

Eine Suchanfrage wird auf dieselbe Weise in Wörter geteilt, und ein Beitrag wird gefunden, wenn er jedes Wort der Suchanfrage hat, das letzte auch als Anfang eines längeren Wortes (`テキ` findet `テキスト`). Nur die ersten 10 Wörter einer Suchanfrage zählen.

Was die Suche nicht finden kann:

- **Eine Zeichenfolge, die kein Wort der Segmentierung des Wörterbuchs ist.** Die Suche findet Wörter, keine Teilstrings: `京都` findet `東京都` nicht, und ein Teil, den das Wörterbuch innerhalb eines längeren Wortes hält, etwa `庁舎` in `東京都庁舎見学`, wird ebenfalls nicht gefunden.
- **Lesungen.** Hiragana findet dasselbe Wort in Katakana (`かたかな` findet `カタカナ`), aber eine Lesung findet kein Kanji (`ぜんぶん` findet `全文` nicht).
- **Ähnliche Schreibweisen.** Die Tippfehlertoleranz ist aus, daher muss jedes Wort exakt sein.
- **Japanische Beiträge ohne Sprache.** Ein Beitrag, dessen Sprache fehlt oder falsch ist, wird nach dem segmentiert, was Meilisearch erkennt, und Text nur in Kanji wird dann oft für Chinesisch gehalten und anders geteilt: Ein solcher Beitrag wird womöglich von `京都` für `東京都` gefunden oder nicht von `東京`. Ein Beitrag trägt die Sprache, die seine Autorin oder sein Autor gewählt hat, oder was der Server der Autorin oder des Autors gesendet hat.

Der Server faltet die Kompatibilitätsformen von Unicode in Beiträgen und Suchanfragen, bevor sie Meilisearch erreichen, sodass halbbreite Katakana (`ｶﾀｶﾅ`) und vollbreite Buchstaben und Ziffern (`ＡＢＣ`) ihre üblichen Formen finden und von ihnen gefunden werden. Auch Emoji sind Wörter: `👍` findet die Beiträge, die es haben.

Die Suche braucht für nichts davon ein eigenes Plugin, Wörterbuch oder Image: Das offizielle Image hat die Wörterbücher für Japanisch, Chinesisch und Koreanisch.
