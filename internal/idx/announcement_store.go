package idx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	. "github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/table"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type AnnouncementStore interface {
	Announcement(ctx context.Context, db qrm.DB, id ...string) ([]model.Announcements, error)
	InsertAnnouncement(ctx context.Context, db qrm.DB, ann ...model.Announcements) error
	LatestAnnouncement(ctx context.Context, db qrm.DB) (model.Announcements, error)
	IsExists(ctx context.Context, db qrm.DB) (bool, error)
	IsProcessed(ctx context.Context, db qrm.DB, idxIDs ...string) (map[string]bool, error)
}

func NewAnnouncementStore(db *sql.DB, logger *slog.Logger, tracer trace.Tracer) AnnouncementStore {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "state.AnnouncementStore"))
	}
	return &announcementStore{db: db, logger: logger, tracer: tracer}
}

type announcementStore struct {
	db     *sql.DB
	logger *slog.Logger
	tracer trace.Tracer
}

func (s *announcementStore) IsExists(ctx context.Context, db qrm.DB) (bool, error) {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"idx.announcementStore.isExists",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.announcementStore.IsExists"))

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := SELECT(EXISTS(Announcements.SELECT(Announcements.AllColumns).LIMIT(1)))
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String(constants.SQLStatement, stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "checking announcements")
	span.AddEvent("checking announcements")
	var isExists struct{ bool }
	if err := stmt.QueryContext(ctx, db, &isExists); err != nil {
		err = fmt.Errorf("checking announcements: %v", err)
		telemetry.RecordError(span, err)
		return false, err
	}
	logger = logger.With(slog.Bool(constants.IsExists, isExists.bool))
	logger.InfoContext(ctx, "checked announcements")
	span.AddEvent("checked announcements")

	return isExists.bool, nil
}

func (s *announcementStore) LatestAnnouncement(
	ctx context.Context,
	db qrm.DB,
) (model.Announcements, error) {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"idx.announcementStore.LatestAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	logger := s.logger.With(
		slog.String("tag", "idx.announcementStore.LatestAnnouncement"),
		slog.Time(constants.CurrentTime, now),
	)

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Announcements.SELECT(Announcements.AllColumns).
		ORDER_BY(Announcements.Date.DESC()).
		LIMIT(1)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String(constants.SQLStatement, stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "get latest announcement")
	span.AddEvent("get latest announcement")
	var ann model.Announcements
	if err := stmt.QueryContext(ctx, db, &ann); err != nil {
		err = fmt.Errorf("get latest announcement: %v", err)
		if !errors.Is(err, qrm.ErrNoRows) {
			telemetry.RecordError(span, err)
		}
		return model.Announcements{}, err
	}
	logger = logger.With(slog.Any(constants.LatestAnnouncement, ann))
	logger.InfoContext(ctx, "got latest announcements")
	span.AddEvent("got latest announcement")

	return ann, nil
}

func (s *announcementStore) IsProcessed(
	ctx context.Context,
	db qrm.DB,
	idxIDs ...string,
) (map[string]bool, error) {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"idx.announcementStore.IsProcessed",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	idxIdsLen := len(idxIDs)
	logger := s.logger.With(
		slog.String("tag", "idx.announcementStore.IsProcessed"),
		slog.Int("ids_count", idxIdsLen),
	)

	if idxIdsLen == 0 {
		return map[string]bool{}, nil
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := SELECT(Announcements.AllColumns).
		FROM(Announcements).
		WHERE(Announcements.IdxID.EQ(ANY(StringArray(idxIDs...))))
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "checking announcements is processed")
	span.AddEvent("checking announcements is processed")
	var announcements []model.Announcements
	if err := stmt.QueryContext(ctx, db, &announcements); err != nil {
		err = fmt.Errorf("checking announcements is processed: %v", err)
		if errors.Is(err, qrm.ErrNoRows) {
			logger.WarnContext(ctx, "", slog.Any("error", err))
			return map[string]bool{}, nil
		}
		telemetry.RecordError(span, err)
		return nil, err
	}
	result := make(map[string]bool, idxIdsLen)
	for _, id := range idxIDs {
		result[id] = false
	}
	for _, ann := range announcements {
		result[ann.IdxID] = true
	}
	annLen := len(announcements)
	logger = logger.With(
		slog.Bool("is_announcements_processed", annLen > 0),
		slog.Int(constants.Found, annLen),
		slog.Int("not_found", idxIdsLen-annLen),
	)
	logger.InfoContext(ctx, "checked announcements is processed")
	span.AddEvent("checked announcements is processed")

	return result, nil
}

func (s *announcementStore) InsertAnnouncement(
	ctx context.Context,
	db qrm.DB,
	ann ...model.Announcements,
) error {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"idx.announcementStore.InsertAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "idx.announcementStore.InsertAnnouncement"),
		slog.Int(constants.Count, len(ann)),
	)

	if len(ann) == 0 {
		return nil
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Announcements.INSERT(Announcements.AllColumns.Except(Announcements.DefaultColumns)).
		ON_CONFLICT().
		DO_NOTHING().
		MODELS(ann).
		RETURNING(Announcements.AllColumns)
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "inserting announcements")
	span.AddEvent("inserting announcements")
	var inserted []model.Announcements
	if err := stmt.QueryContext(ctx, db, &inserted); err != nil {
		err = fmt.Errorf("inserting announcements: %v", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger.InfoContext(
		ctx,
		"inserted announcements",
		slog.Int(constants.InsertedCount, len(inserted)),
	)
	span.AddEvent("inserted announcements")

	return nil
}

func (s *announcementStore) Announcement(
	ctx context.Context,
	db qrm.DB,
	id ...string,
) ([]model.Announcements, error) {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(ctx, "idx.announcementStore.Announcement")
	defer span.End()

	span.AddEvent("getting announcement")

	logger := s.logger.With(slog.String("tag", "idx.announcementStore.Announcement"))

	if len(id) == 0 {
		return []model.Announcements{}, nil
	}

	stmt := Announcements.SELECT(Announcements.AllColumns).
		WHERE(Announcements.ID.EQ(ANY(StringArray(id...))))

	var announcements []model.Announcements
	if err := stmt.QueryContext(ctx, db, &announcements); err != nil {
		err = fmt.Errorf("getting announcement: %v", err)
		telemetry.RecordError(span, err)
		return announcements, err
	}
	logger.InfoContext(ctx, "got announcement")
	span.AddEvent("got announcement")

	return announcements, nil
}
