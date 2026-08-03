package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jorgebuitragor/logday-server/internal/note"
	"github.com/jorgebuitragor/logday-server/internal/task"
)

type store struct {
	db *sql.DB
}

// NewStore wires up the store used by NewHandler. It fans out to each
// domain package's own ChangesSince and merges the results — adding a
// new synced entity means adding one more block below. Reconsider a
// generic helper if this grows past 3-4 entities and the repetition
// starts to hurt (see specs/convenciones-codigo/design.md).
func NewStore(db *sql.DB) *store {
	return &store{db: db}
}

func (s *store) changesSince(ctx context.Context, userID string, since int64) ([]change, error) {
	changes := []change{}

	tasks, err := task.ChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching task changes: %w", err)
	}
	for _, t := range tasks {
		data, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("encoding task %s: %w", t.ID, err)
		}
		changes = append(changes, change{
			Type:      "task",
			ID:        t.ID,
			Seq:       t.Seq,
			Deleted:   t.DeletedAt != nil,
			UpdatedAt: t.UpdatedAt,
			Data:      data,
		})
	}

	notes, err := note.ChangesSince(ctx, s.db, userID, since)
	if err != nil {
		return nil, fmt.Errorf("fetching note changes: %w", err)
	}
	for _, n := range notes {
		data, err := json.Marshal(n)
		if err != nil {
			return nil, fmt.Errorf("encoding note %s: %w", n.ID, err)
		}
		changes = append(changes, change{
			Type:      "note",
			ID:        n.ID,
			Seq:       n.Seq,
			Deleted:   n.DeletedAt != nil,
			UpdatedAt: n.UpdatedAt,
			Data:      data,
		})
	}

	// seq is a single counter shared by every entity of a user (see
	// internal/db.NextSeq), so merging already-per-entity-sorted lists
	// and re-sorting by seq gives a globally consistent order.
	sort.Slice(changes, func(i, j int) bool { return changes[i].Seq < changes[j].Seq })
	return changes, nil
}
