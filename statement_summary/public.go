// Package statement_summary is the nterface to events_stages_summary_global_by_event_name
package statement_summary

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sjmudd/ps-top/baseobject"
	"github.com/sjmudd/ps-top/context"
	"github.com/sjmudd/ps-top/logger"
)

/*

root@localhost [performance_schema]> select EVENT_NAME, COUNT_STAR, SUM_TIMER_WAIT, SUM_ROWS_EXAMINED from events_statements_summary_global_by_event_name order by SUM_TIMER_WAIT DESC LIMIT 20;
+----------------------------------+------------+------------------+-------------------+
| EVENT_NAME                       | COUNT_STAR | SUM_TIMER_WAIT   | SUM_ROWS_EXAMINED |
+----------------------------------+------------+------------------+-------------------+
| statement/sql/select             |    2366024 | 1134010657232000 |          15832115 |
| statement/sql/insert             |     102965 |  358676581245000 |                 0 |
| statement/sql/delete             |     539253 |  156619895487000 |            799185 |
| statement/sql/insert_select      |        268 |   79078085687000 |           1459796 |
| statement/sql/update             |       3482 |   21372903170000 |             75729 |
| statement/scheduler/event        |       1480 |    6245639372000 |                 0 |
| statement/sp/stmt                |       4697 |    5723933199000 |               672 |
| statement/sql/update_multi       |         11 |    5082249447000 |            149831 |
| statement/sql/replace_select     |         22 |    3035530650000 |            126954 |
| statement/com/Quit               |     148486 |    1278334466000 |                 0 |
| statement/sql/call_procedure     |          1 |     383967503000 |                 0 |
| statement/sql/show_status        |        248 |     249204379000 |             32540 |
| statement/sql/delete_multi       |         78 |     239369594000 |             60990 |
| statement/sql/show_variables     |        223 |     214567132000 |              5608 |
| statement/sql/show_engine_status |         91 |     149844880000 |                 0 |
| statement/sql/replace            |         76 |      95745936000 |                 0 |
| statement/sp/set                 |       4470 |      60120912000 |                 0 |
| statement/sql/show_slave_status  |        272 |      32298502000 |                 0 |
| statement/sql/show_master_status |        237 |      20872123000 |                 0 |
| statement/sql/set_option         |        288 |      20497803000 |                 0 |
+----------------------------------+------------+------------------+-------------------+
20 rows in set (0.30 sec)

*/

// Object provides a public view of object
type Object struct {
	baseobject.BaseObject      // embedded
	initial               Rows // initial data for relative values
	current               Rows // last loaded values
	results               Rows // results (maybe with subtraction)
	totals                Row  // totals of results
}

func (t *Object) copyCurrentToInitial() {
	t.initial = make(Rows, len(t.current))
	t.SetInitialCollectTime(t.LastCollectTime())
	copy(t.initial, t.current)
}

func NewStatementSummary(ctx *context.Context) *Object {
	logger.Println("NewStatementSummary()")
	o := new(Object)
	o.SetContext(ctx)

	return o
}

// Collect collects data from the db, updating initial
// values if needed, and then subtracting initial values if we want
// relative values, after which it stores totals.
func (t *Object) Collect(dbh *sql.DB) {
	start := time.Now()
	t.current = selectRows(dbh)
	t.SetLastCollectTimeNow()
	logger.Println("t.current collected", len(t.current), "row(s) from SELECT")

	if len(t.initial) == 0 && len(t.current) > 0 {
		logger.Println("t.initial: copying from t.current (initial setup)")
		t.copyCurrentToInitial()
	}

	// check for reload initial characteristics
	if t.initial.needsRefresh(t.current) {
		logger.Println("t.initial: copying from t.current (data needs refreshing)")
		t.copyCurrentToInitial()
	}

	t.makeResults()

	// logger.Println( "t.initial:", t.initial )
	// logger.Println( "t.current:", t.current )
	logger.Println("t.initial.totals():", t.initial.totals())
	logger.Println("t.current.totals():", t.current.totals())
	// logger.Println("t.results:", t.results)
	// logger.Println("t.totals:", t.totals)
	logger.Println("Table_io_waits_summary_by_table.Collect() END, took:", time.Duration(time.Since(start)).String())
}

// Headings returns the headings of the object
func (t *Object) Headings() string {
	return t.totals.headings()
}

// RowContent returns a slice of strings containing the row content
func (t Object) RowContent() []string {
	rows := make([]string, 0, len(t.results))

	for i := range t.results {
		rows = append(rows, t.results[i].rowContent(t.totals))
	}

	return rows
}

// EmptyRowContent returns an empty row
func (t Object) EmptyRowContent() string {
	var e Row

	return e.rowContent(e)
}

// TotalRowContent returns a row containing the totals
func (t Object) TotalRowContent() string {
	return t.totals.rowContent(t.totals)
}

// Description describe the stages
func (t Object) Description() string {
	var count int
	for row := range t.results {
		if t.results[row].sumTimerWait > 0 {
			count++
		}
	}

	return fmt.Sprintf("SQL Stage Latency (events_stages_summary_global_by_event_name) %d rows", count)
}

// SetInitialFromCurrent  resets the statistics to current values
func (t *Object) SetInitialFromCurrent() {
	t.copyCurrentToInitial()
	t.makeResults()
}

// generate the results and totals and sort data
func (t *Object) makeResults() {
	// logger.Println( "- t.results set from t.current" )
	t.results = make(Rows, len(t.current))
	copy(t.results, t.current)
	if t.WantRelativeStats() {
		t.results.subtract(t.initial)
	}

	t.results.sort()
	t.totals = t.results.totals()
}

// Len returns the length of the result set
func (t Object) Len() int {
	return len(t.results)
}

// HaveRelativeStats is true for this object
func (t Object) HaveRelativeStats() bool {
	return true
}
