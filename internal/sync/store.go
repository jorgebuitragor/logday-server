package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/absence"
	"github.com/jorgebuitragor/logday-server/internal/calendar"
	"github.com/jorgebuitragor/logday-server/internal/dailyentry"
	"github.com/jorgebuitragor/logday-server/internal/db"
	"github.com/jorgebuitragor/logday-server/internal/note"
	"github.com/jorgebuitragor/logday-server/internal/overtime"
	"github.com/jorgebuitragor/logday-server/internal/task"
)

// errCursorInvalid signals that since is older than the tombstone
// purge watermark — some deletes it would need to see are gone, so
// the caller must do a full resync instead of an incremental one.
var errCursorInvalid = errors.New("cursor is no longer valid")

type store struct {
	db *sql.DB
}

// NewStore wires up the store used by NewHandler. It fans out to each
// domain package's own ChangesSince and merges the results — adding a
// new synced entity means adding one more addChanges call below.
func NewStore(db *sql.DB) *store {
	return &store{db: db}
}

// changesSince returns errCursorInvalid if since predates the purge
// watermark for userID — callers must not treat that as "no changes",
// since some tombstones in that range are gone and can't be reported.
//
// limit (0 = unbounded) truncates the merged, seq-sorted result — it
// does not push a LIMIT into each domain's own query, so this doesn't
// reduce the cost of fetching from the 7 tables, only the size of the
// response actually sent. See specs/sync-incremental/design.md
// "Paginación" for why: doing this properly (a real cross-table LIMIT)
// needs a bigger rewrite of the fan-out below, not justified without
// evidence the query itself is the bottleneck.
func (s *store) changesSince(ctx context.Context, userID string, since, limit int64) ([]change, error) {
	if since > 0 {
		purgedBefore, err := db.PurgedBeforeSeq(ctx, s.db, userID)
		if err != nil {
			return nil, fmt.Errorf("checking purge watermark: %w", err)
		}
		if since < purgedBefore {
			return nil, errCursorInvalid
		}
	}

	changes := []change{}
	var err error

	tasks, err := task.ChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching task changes: %w", err)
	}
	changes, err = addChanges(changes, "task", tasks,
		func(t task.Task) string { return t.ID },
		func(t task.Task) int64 { return t.Seq },
		func(t task.Task) bool { return t.DeletedAt != nil },
		func(t task.Task) time.Time { return t.UpdatedAt },
	)
	if err != nil {
		return nil, err
	}

	notes, err := note.ChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching note changes: %w", err)
	}
	changes, err = addChanges(changes, "note", notes,
		func(n note.Note) string { return n.ID },
		func(n note.Note) int64 { return n.Seq },
		func(n note.Note) bool { return n.DeletedAt != nil },
		func(n note.Note) time.Time { return n.UpdatedAt },
	)
	if err != nil {
		return nil, err
	}

	overtimeEntries, err := overtime.EntryChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching overtime entry changes: %w", err)
	}
	changes, err = addChanges(changes, "overtime_entry", overtimeEntries,
		func(e overtime.Entry) string { return e.ID },
		func(e overtime.Entry) int64 { return e.Seq },
		func(e overtime.Entry) bool { return e.DeletedAt != nil },
		func(e overtime.Entry) time.Time { return e.UpdatedAt },
	)
	if err != nil {
		return nil, err
	}

	monthMeta, err := overtime.MonthMetaChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching overtime month meta changes: %w", err)
	}
	changes, err = addChanges(changes, "overtime_month_meta", monthMeta,
		func(m overtime.MonthMeta) string { return m.YearMonth },
		func(m overtime.MonthMeta) int64 { return m.Seq },
		func(m overtime.MonthMeta) bool { return m.DeletedAt != nil },
		func(m overtime.MonthMeta) time.Time { return m.UpdatedAt },
	)
	if err != nil {
		return nil, err
	}

	events, err := calendar.ChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching calendar event changes: %w", err)
	}
	changes, err = addChanges(changes, "calendar_event", events,
		func(e calendar.Event) string { return e.ID },
		func(e calendar.Event) int64 { return e.Seq },
		func(e calendar.Event) bool { return e.DeletedAt != nil },
		func(e calendar.Event) time.Time { return e.UpdatedAt },
	)
	if err != nil {
		return nil, err
	}

	absenceDays, err := absence.ChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching absence day changes: %w", err)
	}
	changes, err = addChanges(changes, "absence_day", absenceDays,
		func(d absence.Day) string { return d.ID },
		func(d absence.Day) int64 { return d.Seq },
		func(d absence.Day) bool { return d.DeletedAt != nil },
		func(d absence.Day) time.Time { return d.UpdatedAt },
	)
	if err != nil {
		return nil, err
	}

	dailyEntries, err := dailyentry.ChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching daily entry changes: %w", err)
	}
	changes, err = addChanges(changes, "daily_entry", dailyEntries,
		func(e dailyentry.Entry) string { return e.Date },
		func(e dailyentry.Entry) int64 { return e.Seq },
		func(e dailyentry.Entry) bool { return e.DeletedAt != nil },
		func(e dailyentry.Entry) time.Time { return e.UpdatedAt },
	)
	if err != nil {
		return nil, err
	}

	// seq is a single counter shared by every entity of a user (see
	// internal/db.NextSeq), so merging already-per-entity-sorted lists
	// and re-sorting by seq gives a globally consistent order.
	sort.Slice(changes, func(i, j int) bool { return changes[i].Seq < changes[j].Seq })
	if limit > 0 && int64(len(changes)) > limit {
		changes = changes[:limit]
	}
	return changes, nil
}

// addChanges JSON-encodes each item and appends it to changes as a
// unified change envelope. Function parameters (rather than an
// interface every domain type would need to implement) keep this
// generic without touching each domain package's own types.
func addChanges[T any](changes []change, entityType string, items []T, id func(T) string, seq func(T) int64, deleted func(T) bool, updatedAt func(T) time.Time) ([]change, error) {
	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("encoding %s %s: %w", entityType, id(item), err)
		}
		changes = append(changes, change{
			Type:      entityType,
			ID:        id(item),
			Seq:       seq(item),
			Deleted:   deleted(item),
			UpdatedAt: updatedAt(item),
			Data:      data,
		})
	}
	return changes, nil
}
