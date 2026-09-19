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
	InsertAnnouncement(ctx context.Context, db qrm.DB, ann ...model.Announcements) error
	LatestAnnouncement(ctx context.Context, db qrm.DB) (model.Announcements, error)
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
		err = fmt.Errorf("get latest announcement: %w", err)
		if !errors.Is(err, qrm.ErrNoRows) {
			telemetry.RecordError(span, err)
		}
		return model.Announcements{}, err
	}
	logger = logger.With(slog.Any(constants.LatestAnnouncement, ann))
	logger.DebugContext(ctx, "got latest announcements")
	span.AddEvent("got latest announcement")

	return ann, nil
}

func (s *announcementStore) IsProcessed(
	ctx context.Context,
	db qrm.DB,
	idxIDs ...string,
) (map[string]bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"idx.announcementStore.IsProcessed",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	idxIdsLen := len(idxIDs)
	if idxIdsLen == 0 {
		return map[string]bool{}, errors.New("idxIdsLen should not be 0")
	}

	logger := s.logger.With(
		slog.String("tag", "idx.announcementStore.IsProcessed"),
		slog.Int("ids_count", idxIdsLen),
	)

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
		if !errors.Is(err, qrm.ErrNoRows) {
			telemetry.RecordError(span, err)
		}
		if errors.Is(err, qrm.ErrNoRows) {
			return map[string]bool{}, nil
		}
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
	logger.DebugContext(ctx, "checked announcements is processed")
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
		err = fmt.Errorf("inserting announcements: %w", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger = logger.With(slog.Int(constants.InsertedCount, len(inserted)))
	logger.DebugContext(ctx, "inserted announcements")
	span.AddEvent("inserted announcements")

	return nil
}
