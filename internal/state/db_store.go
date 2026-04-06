package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"

	. "github.com/alturino/bloodhound/internal/db/.gen/postgres/public/table"
	"github.com/alturino/bloodhound/internal/models"
)

// DBStore implements the Store interface using PostgreSQL and go-jet
type DBStore struct {
	db *sql.DB
}

func (s *DBStore) HasSavedAnnouncements(ctx context.Context) (bool, error) {
	var ann models.Announcement
	stmt := SELECT(Announcements.AllColumns).FROM(Announcements).LIMIT(1)
	err := stmt.QueryContext(ctx, s.db, &ann)
	if err != nil {
		if errors.Is(err, qrm.ErrNoRows){
			return false, nil
		}
		return false, fmt.Errorf("DBStore.HasSavedAnnouncements: %w", err)
	}
	return true, nil
}

// NewDBStore creates a new PostgreSQL-backed store
func NewDBStore(db *sql.DB) *DBStore {
	return &DBStore{db: db}
}

// IsProcessed checks if an announcement ID exists in the database by its IDX ID
func (s *DBStore) IsProcessed(ctx context.Context, idxID string) (bool, error) {
	var isExist bool
	isExistStmt := SELECT(Int(1)).FROM(Announcements).WHERE(Announcements.IdxID.EQ(String(idxID)))
	if err := SELECT(EXISTS(isExistStmt)).QueryContext(ctx, s.db, &isExist); err != nil {
		return false, fmt.Errorf("DBStore.IsProcessed: %w", err)
	}
	return isExist, nil
}

// RecordAnnouncement saves announcement metadata
func (s *DBStore) RecordAnnouncement(ctx context.Context, ann models.Announcement) error {
	announcement := ann.ToAnnouncements()
	if err := Announcements.INSERT(Announcements.AllColumns.Except(Announcements.ID, Announcements.CreatedAt)).
		MODEL(announcement).
		RETURNING(Announcements.AllColumns).
		QueryContext(ctx, s.db, &announcement); err != nil {
		return err
	}
	return nil
}

// RecordAttachment saves attachment metadata
func (s *DBStore) RecordAttachment(
	ctx context.Context,
	idxID string,
	att models.Attachment,
	checksum string,
	storagePath string,
) error {
	attachment := att.ToAttachments(idxID, checksum, storagePath)
	if err := Attachments.INSERT(Attachments.AllColumns.Except(Attachments.ID)).
		MODEL(attachment).
		RETURNING(Attachments.AllColumns).
		QueryContext(ctx, s.db, &attachment); err != nil {
		return fmt.Errorf("DBStore.RecordAttachment: %w", err)
	}
	return nil
}
