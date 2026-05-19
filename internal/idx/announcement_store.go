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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	. "github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/table"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type AnnouncementStore interface {
	Announcement(ctx context.Context, id ...string) ([]model.Announcements, error)
	InsertAnnouncement(ctx context.Context, tx qrm.DB, ann ...model.Announcements) error
	LatestAnnouncement(ctx context.Context) (model.Announcements, error)
	IsExists(ctx context.Context) (bool, error)
	IsProcessed(ctx context.Context, idxIDs ...string) (map[string]bool, error)
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

func (s *announcementStore) IsExists(ctx context.Context) (bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"store.announcementStore.isExists",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("tag", "store.announcementStore.IsExists")),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "store.announcementStore.IsExists"))

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	isEmptyStmt := SELECT(EXISTS(Announcements.SELECT(Announcements.AllColumns).LIMIT(1)))
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", isEmptyStmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "checking announcements")
	span.AddEvent("checking announcements")
	var isExists struct{ bool }
	if err := isEmptyStmt.QueryContext(ctx, s.db, &isExists); err != nil {
		err = fmt.Errorf("checking announcements: %w", err)
		telemetry.RecordError(span, err)
		return false, err
	}
	logger = logger.With(slog.Bool("is_exists", isExists.bool))
	logger.InfoContext(ctx, "checked announcements")
	span.AddEvent("checked announcements")

	return isExists.bool, nil
}

func (s *announcementStore) LatestAnnouncement(ctx context.Context) (model.Announcements, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"store.announcementStore.LatestAnnouncement",
		trace.WithAttributes(attribute.String("tag", "store.announcementStore.LatestAnnouncement")),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	logger := s.logger.With(
		slog.String("tag", "store.announcementStore.LatestAnnouncement"),
		slog.Time("current_time", now),
	)

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Announcements.SELECT(Announcements.AllColumns).
		ORDER_BY(Announcements.Date.DESC()).
		LIMIT(1)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "get latest announcement")
	span.AddEvent("get latest announcement")
	var ann model.Announcements
	if err := stmt.QueryContext(ctx, s.db, &ann); err != nil {
		err = fmt.Errorf("get latest announcement: %w", err)
		telemetry.RecordError(span, err)
		return model.Announcements{}, err
	}
	logger.InfoContext(ctx, "got latest announcements", slog.Any("latest_announcement", ann))
	span.AddEvent("got latest announcements")

	return ann, nil
}

func (s *announcementStore) IsProcessed(ctx context.Context, idxIDs ...string) (map[string]bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"store.announcementStore.IsProcessed",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.announcementStore.IsProcessed"),
		slog.Int("count", len(idxIDs)),
	)

	if len(idxIDs) == 0 {
		logger.DebugContext(ctx, "announcements empty returning")
		span.AddEvent("announcements empty returning")
		return map[string]bool{}, nil
	}

	logger.DebugContext(ctx, "single check processed")
	span.AddEvent("single check processed")

	stmt := SELECT(Announcements.AllColumns).
		FROM(Announcements).
		WHERE(Announcements.IdxID.EQ(ANY(StringArray(idxIDs...))))
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}

	logger.DebugContext(ctx, "is processed")
	span.AddEvent("is processed")
	var announcements []model.Announcements
	if err := stmt.QueryContext(ctx, s.db, &announcements); err != nil {
		err = fmt.Errorf("is processed: %w", err)
		if errors.Is(err, qrm.ErrNoRows) {
			logger.WarnContext(ctx, "announcement not processed", slog.Any("error", err))
			return map[string]bool{}, nil
		}
		logger.ErrorContext(ctx, "is processed", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return nil, err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.Any("processed_announcements", announcements))
	}
	logger.DebugContext(ctx, "is processed")
	span.AddEvent("is processed")

	result := make(map[string]bool, len(idxIDs))
	for _, id := range idxIDs {
		result[id] = false
	}
	for _, ann := range announcements {
		result[ann.IdxID] = true
	}

	logger.DebugContext(ctx, "batch checked processed", slog.Int("found", len(announcements)))
	span.AddEvent("batch checked processed")

	return result, nil
}

func (s *announcementStore) InsertAnnouncement(
	ctx context.Context,
	tx qrm.DB,
	ann ...model.Announcements,
) error {
	if tx == nil {
		tx = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"store.announcementStore.InsertAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.announcementStore.InsertAnnouncement"),
		slog.Int("count", len(ann)),
	)

	if len(ann) == 0 {
		logger.DebugContext(ctx, "announcements empty returning")
		span.AddEvent("announcements empty returning")
		return nil
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Announcements.INSERT(Announcements.AllColumns.Except(Announcements.DefaultColumns)).
		ON_CONFLICT().
		DO_NOTHING().
		MODELS(ann).
		RETURNING(Announcements.AllColumns)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.InfoContext(ctx, "inserting announcements")
	span.AddEvent("inserting announcements")
	var inserted []model.Announcements
	if err := stmt.QueryContext(ctx, tx, &inserted); err != nil {
		err = fmt.Errorf("inserting announcements: %w", err)
		logger.ErrorContext(ctx, "inserting announcements", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.Any("inserted_announcements", inserted))
	}
	logger.InfoContext(ctx, "inserted announcements", slog.Int("inserted_count", len(inserted)))
	span.AddEvent("inserted announcements")

	return nil
}

func (s *announcementStore) Announcement(ctx context.Context, id ...string) ([]model.Announcements, error) {
	ctx, span := s.tracer.Start(ctx, "store.announcementStore.Announcement")
	defer span.End()

	logger := s.logger.With(slog.String("tag", "store.announcementStore.Announcement"))

	if len(id) == 0 {
		logger.DebugContext(ctx, "id empty returning")
		span.AddEvent("id empty returning")
		return []model.Announcements{}, nil
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Announcements.SELECT(Announcements.AllColumns).
		WHERE(Announcements.ID.EQ(ANY(StringArray(id...))))
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "getting announcement")
	span.AddEvent("getting announcement")
	var announcements []model.Announcements
	if err := stmt.QueryContext(ctx, s.db, &announcements); err != nil {
		err = fmt.Errorf("getting announcement: %w", err)
		logger.ErrorContext(ctx, "getting announcement", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return announcements, err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.Any("db_announcements", announcements))
	}
	logger.InfoContext(ctx, "got announcement")
	span.AddEvent("got announcement")

	return announcements, nil
}