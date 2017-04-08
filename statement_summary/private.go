package statement_summary

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/sjmudd/ps-top/lib"
	"github.com/sjmudd/ps-top/logger"
)

/**************************************************************************

root@localhost [performance_schema]> show create table events_statements_summary_global_by_event_name\G
*************************** 1. row ***************************
       Table: events_statements_summary_global_by_event_name
Create Table: CREATE TABLE `events_statements_summary_global_by_event_name` (
  `EVENT_NAME` varchar(128) NOT NULL,
  `COUNT_STAR` bigint(20) unsigned NOT NULL,
  `SUM_TIMER_WAIT` bigint(20) unsigned NOT NULL,
  `MIN_TIMER_WAIT` bigint(20) unsigned NOT NULL,
  `AVG_TIMER_WAIT` bigint(20) unsigned NOT NULL,
  `MAX_TIMER_WAIT` bigint(20) unsigned NOT NULL,
  `SUM_LOCK_TIME` bigint(20) unsigned NOT NULL,
  `SUM_ERRORS` bigint(20) unsigned NOT NULL,
  `SUM_WARNINGS` bigint(20) unsigned NOT NULL,
  `SUM_ROWS_AFFECTED` bigint(20) unsigned NOT NULL,
  `SUM_ROWS_SENT` bigint(20) unsigned NOT NULL,
  `SUM_ROWS_EXAMINED` bigint(20) unsigned NOT NULL,
  `SUM_CREATED_TMP_DISK_TABLES` bigint(20) unsigned NOT NULL,
  `SUM_CREATED_TMP_TABLES` bigint(20) unsigned NOT NULL,
  `SUM_SELECT_FULL_JOIN` bigint(20) unsigned NOT NULL,
  `SUM_SELECT_FULL_RANGE_JOIN` bigint(20) unsigned NOT NULL,
  `SUM_SELECT_RANGE` bigint(20) unsigned NOT NULL,
  `SUM_SELECT_RANGE_CHECK` bigint(20) unsigned NOT NULL,
  `SUM_SELECT_SCAN` bigint(20) unsigned NOT NULL,
  `SUM_SORT_MERGE_PASSES` bigint(20) unsigned NOT NULL,
  `SUM_SORT_RANGE` bigint(20) unsigned NOT NULL,
  `SUM_SORT_ROWS` bigint(20) unsigned NOT NULL,
  `SUM_SORT_SCAN` bigint(20) unsigned NOT NULL,
  `SUM_NO_INDEX_USED` bigint(20) unsigned NOT NULL,
  `SUM_NO_GOOD_INDEX_USED` bigint(20) unsigned NOT NULL
) ENGINE=PERFORMANCE_SCHEMA DEFAULT CHARSET=utf8
1 row in set (0.00 sec)

**************************************************************************/

// Row contains the information in one row
type Row struct {
	name         string
	countStar    uint64
        sumRowsExamined uint64
	sumTimerWait uint64
}

// Rows contains a slice of Rows
type Rows []Row

// select the rows into table
func selectRows(dbh *sql.DB) Rows {
	var t Rows

	logger.Println("events_stages_summary_global_by_event_name.selectRows()")
	sql := "SELECT EVENT_NAME, COUNT_STAR, SUM_ROWS_EXAMINED, SUM_TIMER_WAIT FROM events_statements_summary_global_by_event_name WHERE SUM_TIMER_WAIT > 0"

	rows, err := dbh.Query(sql)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var r Row
		if err := rows.Scan(
			&r.name,
			&r.countStar,
			&r.sumRowsExamined,
			&r.sumTimerWait); err != nil {
			log.Fatal(err)
		}

		// convert the stage name, removing any leading stage/sql/
		if len(r.name) > 10 && r.name[0:10] == "statement/" {
			r.name = r.name[10:]
		}

		t = append(t, r)
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
	logger.Println("recovered", len(t), "row(s):")
	logger.Println(t)

	return t
}

// if the data in t2 is "newer", "has more values" than t then it needs refreshing.
// check this by comparing totals.
func (rows Rows) needsRefresh(otherRows Rows) bool {
	myTotals := rows.totals()
	otherTotals := otherRows.totals()

	return myTotals.sumTimerWait > otherTotals.sumTimerWait
}

// generate the totals of a table
func (rows Rows) totals() Row {
	var totals Row
	totals.name = "Totals"

	for i := range rows {
		totals.add(rows[i])
	}

	return totals
}

// add the values of one row to another one
func (row *Row) add(other Row) {
	row.sumTimerWait += other.sumTimerWait
	row.sumRowsExamined += other.sumRowsExamined
	row.countStar += other.countStar
}

// subtract the countable values in one row from another
func (row *Row) subtract(other Row) {
	// check for issues here (we have a bug) and log it
	// - this situation should not happen so there's a logic bug somewhere else
	if row.sumTimerWait >= other.sumTimerWait {
		row.sumTimerWait -= other.sumTimerWait
		row.sumRowsExamined -= other.sumRowsExamined
		row.countStar -= other.countStar
	} else {
		logger.Println("WARNING: Row.subtract() - subtraction problem! (not subtracting)")
		logger.Println("row=", row)
		logger.Println("other=", other)
	}
}

func (rows Rows) Len() int      { return len(rows) }
func (rows Rows) Swap(i, j int) { rows[i], rows[j] = rows[j], rows[i] }

// sort by value (descending) but also by "name" (ascending) if the values are the same
func (rows Rows) Less(i, j int) bool {
	return (rows[i].sumTimerWait > rows[j].sumTimerWait) ||
		((rows[i].sumTimerWait == rows[j].sumTimerWait) && (rows[i].name < rows[j].name))
}

func (rows Rows) sort() {
	sort.Sort(rows)
}

// remove the initial values from those rows where there's a match
// - if we find a row we can't match ignore it
func (rows *Rows) subtract(initial Rows) {
	initialByName := make(map[string]int)

	// iterate over rows by name
	for i := range initial {
		initialByName[initial[i].name] = i
	}

	for i := range *rows {
		name := (*rows)[i].name
		if _, ok := initialByName[name]; ok {
			initialIndex := initialByName[name]
			(*rows)[i].subtract(initial[initialIndex])
		}
	}
}

// stage headings
func (row *Row) headings() string {
	return fmt.Sprintf("%10s %6s %6s %8s|%s", "Latency", "%", "Rows Examined", "Counter", "Statement")
}

// generate a printable result
func (row *Row) rowContent(totals Row) string {
	name := row.name
	if row.countStar == 0 && name != "Totals" {
		name = ""
	}

	return fmt.Sprintf("%10s %6s %s %8s|%s",
		lib.FormatTime(row.sumTimerWait),
		lib.FormatPct(lib.MyDivide(row.sumTimerWait, totals.sumTimerWait)),
		lib.FormatAmount(row.countStar),
		name)
}

// String describes a whole row
func (row Row) String() string {
	return fmt.Sprintf("%10s %10s %10s %s",
		lib.FormatTime(row.sumTimerWait),
		lib.FormatTime(row.sumRowsExamined),
		lib.FormatAmount(row.countStar),
		row.name)
}

// String describes a whole table
func (rows Rows) String() string {
	s := make([]string, len(rows))

	for i := range rows {
		s = append(s, rows[i].String())
	}

	return strings.Join(s, "\n")
}
