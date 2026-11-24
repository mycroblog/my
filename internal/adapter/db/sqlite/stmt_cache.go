// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"container/list"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"

	msqlite "modernc.org/sqlite"
)

// stmtCacheSize is how many prepared statements each connection keeps, the least recently used
// going first. The statements of the requests that a server answers most are a few hundred; a
// statement takes a few kilobytes of SQLite's memory per connection.
const stmtCacheSize = 512

// The driver parses and plans a statement whenever database/sql hands it the SQL of a query
// (sqlite3_prepare_v2), which under concurrent requests took over a quarter of the server's CPU
// (#3183). A connection of cachingConnector keeps the statements it prepared and runs the same
// SQL again on its prepared statement, which the driver resets and binds anew. database/sql uses
// a connection from one goroutine at a time, so a connection's cache needs no lock.

// cachingConnector opens the driver's connections with a cache of prepared statements each.
type cachingConnector struct{ base driver.Connector }

func newCachingConnector(dsn string) (driver.Connector, error) {
	base, err := msqlite.NewConnector(dsn)
	if err != nil {
		return nil, err
	}
	return cachingConnector{base: base}, nil
}

// Connect implements driver.Connector.
func (c cachingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return newCachingConn(conn)
}

// Driver implements driver.Connector.
func (c cachingConnector) Driver() driver.Driver { return c.base.Driver() }

// The interfaces of the driver's connections that cachingConn forwards (msqlite.NewConnector).
type driverConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
	driver.SessionResetter
	driver.Validator
}

// The interfaces of the driver's statements that cachingConn runs them through.
type driverStmt interface {
	driver.Stmt
	driver.StmtExecContext
	driver.StmtQueryContext
}

// cachingConn is a connection of the driver with its prepared statements by SQL.
type cachingConn struct {
	driverConn
	byQuery map[string]*list.Element // of *cachedStmt
	lru     *list.List               // most recently used first
}

// cachedStmt is one prepared statement of a connection; busy while rows of it are open, when the
// same SQL runs on a statement of its own instead.
type cachedStmt struct {
	query string
	stmt  driverStmt
	busy  bool
}

func newCachingConn(conn driver.Conn) (driver.Conn, error) {
	dc, ok := conn.(driverConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("sqlite: the driver's connection %T lacks an interface the adapter forwards", conn)
	}
	return &cachingConn{driverConn: dc, byQuery: map[string]*list.Element{}, lru: list.New()}, nil
}

// Unwrap returns the driver's connection, as msqlite.NewConnector asks of a wrapper.
func (c *cachingConn) Unwrap() driver.Conn { return c.driverConn }

// stmt returns the prepared statement of query to run now: the cached one unless it is busy, and
// whether it is cached, so that the caller closes one that is not.
func (c *cachingConn) stmt(ctx context.Context, query string) (*cachedStmt, bool, error) {
	if e, ok := c.byQuery[query]; ok {
		cs := e.Value.(*cachedStmt)
		if !cs.busy {
			c.lru.MoveToFront(e)
			return cs, true, nil
		}
		s, err := c.prepare(ctx, query)
		return &cachedStmt{query: query, stmt: s}, false, err
	}
	s, err := c.prepare(ctx, query)
	if err != nil {
		return nil, false, err
	}
	cs := &cachedStmt{query: query, stmt: s}
	c.byQuery[query] = c.lru.PushFront(cs)
	for c.lru.Len() > stmtCacheSize {
		c.evict(c.lru.Back())
	}
	return cs, true, nil
}

func (c *cachingConn) prepare(ctx context.Context, query string) (driverStmt, error) {
	s, err := c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	ds, ok := s.(driverStmt)
	if !ok {
		_ = s.Close()
		return nil, fmt.Errorf("sqlite: the driver's statement %T cannot run with a context", s)
	}
	return ds, nil
}

// evict drops e from the cache and closes its statement, unless rows of it are open: those
// close it (cachedRows).
func (c *cachingConn) evict(e *list.Element) {
	cs := e.Value.(*cachedStmt)
	c.lru.Remove(e)
	delete(c.byQuery, cs.query)
	if !cs.busy {
		_ = cs.stmt.Close()
	}
}

// ExecContext implements driver.ExecerContext on the prepared statement of query.
func (c *cachingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	cs, cached, err := c.stmt(ctx, query)
	if err != nil {
		return nil, err
	}
	res, err := cs.stmt.ExecContext(ctx, args)
	if !cached {
		err = errors.Join(err, cs.stmt.Close())
	}
	return res, err
}

// QueryContext implements driver.QueryerContext on the prepared statement of query, which stays
// busy until the rows close.
func (c *cachingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	cs, cached, err := c.stmt(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := cs.stmt.QueryContext(ctx, args)
	if err != nil {
		if !cached {
			_ = cs.stmt.Close()
		}
		return nil, err
	}
	cs.busy = true
	return &cachedRows{Rows: rows, conn: c, cs: cs, cached: cached}, nil
}

// Close closes the cached statements, then the connection.
func (c *cachingConn) Close() error {
	var errs []error
	for e := c.lru.Front(); e != nil; e = e.Next() {
		if cs := e.Value.(*cachedStmt); !cs.busy {
			errs = append(errs, cs.stmt.Close())
		}
	}
	c.byQuery, c.lru = nil, list.New()
	return errors.Join(append(errs, c.driverConn.Close())...)
}

// cachedRows are the rows of a statement of cachingConn; closing them frees the statement for the
// next query, or closes it when it is no longer cached.
type cachedRows struct {
	driver.Rows
	conn   *cachingConn
	cs     *cachedStmt
	cached bool
	closed bool
}

// Close implements driver.Rows.
func (r *cachedRows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	err := r.Rows.Close()
	r.cs.busy = false
	if e, ok := r.conn.byQuery[r.cs.query]; !r.cached || !ok || e.Value != r.cs {
		err = errors.Join(err, r.cs.stmt.Close())
	}
	return err
}

var (
	_ driverConn       = (*cachingConn)(nil)
	_ driver.Rows      = (*cachedRows)(nil)
	_ driver.Connector = cachingConnector{}
)
