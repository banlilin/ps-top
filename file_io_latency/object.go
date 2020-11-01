// Package file_io_latency holds the routines which manage the file_summary_by_instance table.
package file_io_latency

import (
	"database/sql"
	"fmt"

	"github.com/sjmudd/ps-top/baseobject"
	"github.com/sjmudd/ps-top/context"
	"github.com/sjmudd/ps-top/logger"
)

// Object represents the contents of the data collected from file_summary_by_instance
type Object struct {
	baseobject.BaseObject // embedded
	initial               Rows
	current               Rows
	results               Rows
	totals                Row
	db                    *sql.DB
}

// NewFileSummaryByInstance creates a new structure and include various variable values:
// - datadir, relay_log
// There's no checking that these are actually provided!
func NewFileSummaryByInstance(ctx *context.Context, db *sql.DB) *Object {
	logger.Println("NewFileSummaryByInstance()")
	n := &Object{
		db: db,
	}
	n.SetContext(ctx)

	return n
}

// SetInitialFromCurrent resets the statistics to current values
func (t *Object) SetInitialFromCurrent() {
	t.copyCurrentToInitial()

	t.makeResults()
}

func (t *Object) copyCurrentToInitial() {
	t.initial = make(Rows, len(t.current))
	t.SetInitialCollectTime(t.LastCollectTime())
	copy(t.initial, t.current)
}

// Collect data from the db, then merge it in.
func (t *Object) Collect() {
	t.current = selectRows(t.db).mergeByName(t.Variables())
	t.SetLastCollectTimeNow()

	// copy in initial data if it was not there
	if len(t.initial) == 0 && len(t.current) > 0 {
		t.copyCurrentToInitial()
	}

	// check for reload initial characteristics
	if t.initial.needsRefresh(t.current) {
		t.copyCurrentToInitial()
	}

	t.makeResults()
}

func (t *Object) makeResults() {
	t.results = make(Rows, len(t.current))
	copy(t.results, t.current)
	if t.WantRelativeStats() {
		t.results.subtract(t.initial)
	}

	t.results.sort()
	t.totals = t.results.totals()
}

// Headings returns the headings for a table
func (t Object) Headings() string {
	var r Row

	return r.headings()
}

// RowContent returns the rows we need for displaying
func (t Object) RowContent() []string {
	rows := make([]string, 0, len(t.results))

	for i := range t.results {
		rows = append(rows, t.results[i].rowContent(t.totals))
	}

	return rows
}

// Len return the length of the result set
func (t Object) Len() int {
	return len(t.results)
}

// TotalRowContent returns all the totals
func (t Object) TotalRowContent() string {
	return t.totals.rowContent(t.totals)
}

// EmptyRowContent returns an empty string of data (for filling in)
func (t Object) EmptyRowContent() string {
	var empty Row
	return empty.rowContent(empty)
}

// Description returns a description of the table
func (t Object) Description() string {
	var count int
	for row := range t.results {
		if t.results[row].sumTimerWait > 0 {
			count++
		}
	}

	return fmt.Sprintf("File I/O Latency (file_summary_by_instance) %4d row(s)    ", count)
}

// HaveRelativeStats is true for this object
func (t Object) HaveRelativeStats() bool {
	return true
}
