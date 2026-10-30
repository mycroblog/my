# Desired-state schema

The database schema of each dialect (ADR-0023, ADR-0126). `my schema apply` embeds it, diffs it against the target database
with the Atlas Go library (Community Edition), and applies the difference (`scripts/task.sh db:apply DB_DIALECT=sqlite|postgres`;
for PostgreSQL, against the `public` schema of the target database). The Atlas CLI checks changes to it (`db:lint`) and that
nothing is left to apply (`db:check`). Never write migration SQL by hand.

## Layout

- `sqlite/` and `postgres/` hold one file per module, `<NN>-<module>.sql` (ADR-0082). The modules and their numbers are those
  of [data-model §4](../docs/architecture/data-model/04-tables/README.md); `10-infrastructure.sql` holds the tables that
  adapters own.
- Atlas (`file://schema/<dialect>`), sqlc (`sqlc.yaml`), and the Go embedding (`schema.go`) read the files in name order, so a
  statement may only use what an earlier statement or file creates: an index or comment goes after its table. Number a new
  module's file after the files it depends on.
- A table, its indexes, and its comments go in the file of the module that owns the table, also when an index serves
  another module's queries.
- Edit the same file in every dialect together: both directories have the same files, and each file declares the same
  tables in the same order (`schema_test.go`).
- `postgres.bootstrap.sql` runs before the schema on every PostgreSQL database: `my schema apply` runs it on the target,
  and `scripts/task.sh db:up` on Atlas's dev databases.

## Conventions

Tables follow [the logical data model](../docs/architecture/data-model/README.md), mapping logical types per its
[§1.2](../docs/architecture/data-model/01-conventions.md#12-論理型).

SQLite:

- id, string, text, iri, lang, and enum are TEXT; timestamp is INTEGER Unix milliseconds; bool is INTEGER 0/1; bytes is
  BLOB. Every table is STRICT.

PostgreSQL:

- id is uuid (ADR-0078; its byte order is time order for UUIDv7, and Go sees it as a string), except in the columns that
  hold a value which is not always a UUID the server generated (provider and webhook identifiers, client_id, audit
  target_id, emoji_key, run keys, cursors), which stay text COLLATE "C"; string, text, iri, lang, and enum are text;
  timestamp is timestamptz; bool is boolean; bytes is bytea.
- Atlas Community Edition does not manage extensions, sequences, views, or triggers; keep them out of the schema.
  Extensions go in `postgres.bootstrap.sql`.

## Changing the schema

From `v0.1.0`, each pull request that changes the schema must upgrade a database of the previous `main` without losing
data (ADR-0087). Before `v0.1.0` a database may still be recreated, but the same check applies: a destructive change
needs the decider's approval. Classify the change first:

| Class | Examples | How |
| --- | --- | --- |
| A. Additive | A new table or index; a new column that is nullable or has a `DEFAULT`; a relaxed constraint (a dropped `CHECK`, a new value in the list of an enum's `CHECK`) | Change the schema in one pull request |
| B. Projection rebuild | A changed, dropped, or rebuilt column or table of a projection (ADR-0020) | One pull request with the decider's approval; the upgrade rebuilds the projection from its events (`my projection rebuild`) |
| C. Data loss or rewrite | A dropped column or table of a resource, event, or current-value table; a changed type; a split column; a `NOT NULL`, `UNIQUE`, or `CHECK` that existing rows may not satisfy | Never in one pull request: split it in two, below |

The two steps of C:

1. **Expand** (release N): add the new tables or columns as in A. The server writes to and reads from the new side, and an
   upgrade step of `my db upgrade` rewrites the existing rows (keyed, idempotent, in small transactions). A column that
   must be `NOT NULL` gets a `DEFAULT` or goes in a new table, so that the previous release still writes while the new
   schema is applied before the new server starts.
2. **Contract** (a later minor release): remove the old columns or tables from the schema. The release names the upgrade
   steps it relies on, and needs the decider's approval.

### db:lint

`scripts/task.sh db:lint` (`scripts/db-lint.sh`) compares `schema/<dialect>/` of the working tree with the same directory
at the merge base of `BASE` (default `origin/main`) and `HEAD`. It writes the migrations empty → base and base → working tree into a temporary
directory with `atlas migrate diff` and lints the second with `atlas migrate lint`, which also runs it on a dev database;
no migration file stays in the repository. It fails when:

- the lint reports any diagnostic, warnings included: a dropped table or column (a renamed column is a drop and an add),
  `NOT NULL` on an existing column or a new one without `DEFAULT`, a new unique index;
- `atlas schema inspect` shows a changed type of an existing column (Atlas copies the rows or lets PostgreSQL cast them
  without a diagnostic), or a new `CHECK` on an existing table that names an existing column (a `CHECK` on new columns
  only, and a list of an enum's values that grows, are additive);
- the change does not apply on top of the base, such as a type PostgreSQL cannot cast.

```sh
scripts/task.sh db:lint DB_DIALECT=sqlite
scripts/task.sh db:up                          # PostgreSQL: the dev database my_lint of the container
scripts/task.sh db:lint DB_DIALECT=postgres
scripts/task.sh db:lint-test DB_DIALECT=sqlite # the lint itself, on the fixtures of testdata/lint/<dialect>/
```

CI runs it for both dialects:

- **Pull requests** that change the schema (the `db-lint` job) compare with the base branch. A destructive change fails
  unless the pull request has the label **`db:destructive`**, which only the decider adds, for a class B change or a
  contract. Adding or removing the label reruns the checks.
- **The merge queue** compares with the base of the merge group and does not check the label again (a merge group
  carries no labels): it reports destructive changes and fails only when the schema does not apply on top of that base.

A class C change in one pull request is never approved: split it into expand and contract instead.
