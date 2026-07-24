package pubengine

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"
	"time"
)

type interruptedDriver struct{}
type interruptedConn struct{}
type interruptedRows struct{ sent bool }

func (interruptedDriver) Open(string) (driver.Conn, error)  { return interruptedConn{}, nil }
func (interruptedConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (interruptedConn) Close() error                        { return nil }
func (interruptedConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }
func (interruptedConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &interruptedRows{}, nil
}
func (*interruptedRows) Columns() []string {
	return []string{"slug", "title", "date", "tags", "summary", "content", "published", "revision"}
}
func (*interruptedRows) Close() error { return nil }
func (r *interruptedRows) Next(values []driver.Value) error {
	if r.sent {
		return io.ErrUnexpectedEOF
	}
	r.sent = true
	copy(values, []driver.Value{"post", "Post", "2026-01-01", ",go,", "summary", "body", int64(1), int64(1)})
	return nil
}

func init() { sql.Register("pubengine-interrupted-rows", interruptedDriver{}) }

func TestPostIterationErrorsAreNotCached(t *testing.T) {
	db, err := sql.Open("pubengine-interrupted-rows", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Store{db: db}
	if _, err := s.ListPosts(""); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if _, err := s.ListAllPosts(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	c := NewPostCache(s, time.Hour)
	if _, err := c.ListPosts(""); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if c.valid() {
		t.Fatal("incomplete listing was cached")
	}
}
