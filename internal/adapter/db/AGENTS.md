# PostgreSQL (pgx v5), SQLite (modernc.org/sqlite), and the Atlas library

- Changing the isolation level or the retries of a transaction on PostgreSQL: [PostgreSQL documentation](https://www.postgresql.org/docs/current/), search for: Transaction Isolation, Read Committed, `serialization_failure`, `deadlock_detected`
- Writing an upsert or a lock in PostgreSQL: [PostgreSQL documentation](https://www.postgresql.org/docs/current/), search for: `ON CONFLICT`, `pg_advisory_xact_lock`, `SKIP LOCKED`
- Calling pgx (pool, rows, tracing): [pgx v5 package documentation](https://pkg.go.dev/github.com/jackc/pgx/v5), search for: `CollectRows`, `RowTo`, `QueryTracer`, `ErrNoRows`, `pgxpool`
- Changing the journal mode, the busy timeout, or how a transaction begins on SQLite: [SQLite documentation](https://www.sqlite.org/docs.html), search for: Write-Ahead Logging, `busy_timeout`, `BEGIN IMMEDIATE`, `SQLITE_BUSY`
- Writing an upsert or relying on a type in SQLite: [SQLite documentation](https://www.sqlite.org/docs.html), search for: UPSERT, STRICT Tables, Datatypes In SQLite
- Changing the connection string of the SQLite driver: [modernc.org/sqlite package documentation](https://pkg.go.dev/modernc.org/sqlite), search for: `_pragma`, `_txlock`, `_time_format`
- Changing how the schema is diffed, planned, or refused when destructive: [Atlas Go packages documentation](https://pkg.go.dev/ariga.io/atlas/sql/migrate), search for: `Plan`, `PlanChanges`, `DropTable`, `DropColumn`
