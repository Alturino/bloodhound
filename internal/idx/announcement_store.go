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

	span.AddEvent("checking announcements")

	stmt := SELECT(EXISTS(Announcements.SELECT(Announcements.AllColumns).LIMIT(1)))
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String(constants.SQLStatement, stmt.DebugSql()))
	}

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

	span.AddEvent("getting latest announcement")

	now := time.Now()
	logger := s.logger.With(
		slog.String("tag", "idx.announcementStore.LatestAnnouncement"),
		slog.Time(constants.CurrentTime, now),
	)

	stmt := Announcements.SELECT(Announcements.AllColumns).
		ORDER_BY(Announcements.Date.DESC()).
		LIMIT(1)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String(constants.SQLStatement, stmt.DebugSql()))
	}

	var ann model.Announcements
	if err := stmt.QueryContext(ctx, db, &ann); err != nil {
		err = fmt.Errorf("get latest announcement: %v", err)
		telemetry.RecordError(span, err)
		return model.Announcements{}, err
	}
	logger.InfoContext(ctx, "got latest announcements", slog.Any(constants.LatestAnnouncement, ann))
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

	span.AddEvent("checking processed announcements")

	logger := s.logger.With(
		slog.String("tag", "idx.announcementStore.IsProcessed"),
		slog.Int(constants.Count, len(idxIDs)),
	)

	if len(idxIDs) == 0 {
		return map[string]bool{}, nil
	}

	stmt := SELECT(Announcements.AllColumns).
		FROM(Announcements).
		WHERE(Announcements.IdxID.EQ(ANY(StringArray(idxIDs...))))

	var announcements []model.Announcements
	if err := stmt.QueryContext(ctx, db, &announcements); err != nil {
		err = fmt.Errorf("is processed: %v", err)
		if errors.Is(err, qrm.ErrNoRows) {
			logger.WarnContext(ctx, "announcement not processed", slog.Any("error", err))
			return map[string]bool{}, nil
		}
		telemetry.RecordError(span, err)
		return nil, err
	}

	result := make(map[string]bool, len(idxIDs))
	for _, id := range idxIDs {
		result[id] = false
	}
	for _, ann := range announcements {
		result[ann.IdxID] = true
	}

	logger.DebugContext(
		ctx,
		"batch checked processed",
		slog.Int(constants.Found, len(announcements)),
	)
	span.AddEvent("batch checked processed")

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
