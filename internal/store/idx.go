package store

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
	"github.com/alturino/bloodhound/internal/models"
	"github.com/alturino/bloodhound/internal/telemetry"
)

// IDXStore - IDX announcement state persistence
type IDXStore interface {
	Announcement(ctx context.Context, id ...string) ([]model.Announcements, error)
	InsertAnnouncement(ctx context.Context, tx qrm.DB, ann ...model.Announcements) error
	LatestAnnouncement(ctx context.Context) (model.Announcements, error)
	IsExists(ctx context.Context) (bool, error)
	IsProcessed(ctx context.Context, idxIDs ...string) (map[string]bool, error)
	InsertAttachment(ctx context.Context, tx qrm.DB, attachments ...model.Attachments) error
	Attachment(ctx context.Context, id ...string) (model.Attachments, error)
	UnprocessedAttachments(ctx context.Context) ([]model.Attachments, error)
	ClaimAttachments(ctx context.Context, id ...string) error
	UpdateAttachmentResult(ctx context.Context, attachment model.Attachments) error
}

type AttachmentTask struct {
	AnnouncementID string
	Checksum       string
	Path           string
	Err            string
	Attachment     models.Attachment
}

// NewIDXStore creates a new PostgreSQL-backed store
func NewIDXStore(db *sql.DB, logger *slog.Logger, tracer trace.Tracer) IDXStore {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "state.IDXStore"))
	}
	return &idxStore{db: db, logger: logger, tracer: tracer}
}

// idxStore implements the Store interface using PostgreSQL and go-jet
type idxStore struct {
	db     *sql.DB
	logger *slog.Logger
	tracer trace.Tracer
}

func (s *idxStore) IsExists(ctx context.Context) (bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"store.idxStore.isExists",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("tag", "store.idxStore.IsExists")),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "store.idxStore.IsExists"))

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

func (s *idxStore) LatestAnnouncement(ctx context.Context) (model.Announcements, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"store.idxStore.LatestAnnouncement",
		trace.WithAttributes(attribute.String("tag", "store.idxStore.LatestAnnouncement")),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	logger := s.logger.With(
		slog.String("tag", "store.idxStore.LatestAnnouncement"),
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

// IsProcessed checks if announcement IDs exist in the database by their IDX ID
func (s *idxStore) IsProcessed(ctx context.Context, idxIDs ...string) (map[string]bool, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"store.idxStore.IsProcessed",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.idxStore.IsProcessed"),
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

// InsertAnnouncement saves announcement metadata
func (s *idxStore) InsertAnnouncement(
	ctx context.Context,
	tx qrm.DB,
	ann ...model.Announcements,
) error {
	if tx == nil {
		tx = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"store.idxStore.InsertAnnouncement",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.idxStore.InsertAnnouncement"),
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

// RecordAttachment saves attachment metadata
func (s *idxStore) InsertAttachment(
	ctx context.Context,
	tx qrm.DB,
	attachments ...model.Attachments,
) error {
	if tx == nil {
		tx = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"store.idxStore.InsertAttachment",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.idxStore.InsertAttachment"),
		slog.Int("attachment_size", len(attachments)),
	)

	if len(attachments) == 0 {
		logger.DebugContext(ctx, "attachments empty returning")
		span.AddEvent("attachments empty returning")
		return nil
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Attachments.INSERT(Attachments.AllColumns.Except(Attachments.DefaultColumns)).
		ON_CONFLICT().
		DO_NOTHING().
		MODELS(attachments).
		RETURNING(Attachments.AllColumns)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.InfoContext(ctx, "inserting attachments")
	span.AddEvent("inserting attachments")
	if err := stmt.QueryContext(ctx, tx, &attachments); err != nil {
		err = fmt.Errorf("inserting attachments: %w", err)
		logger.ErrorContext(ctx, "inserting attachments", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.Any("inserted_attachents", attachments))
	}
	logger.InfoContext(ctx, "inserted attachments")
	span.AddEvent("inserted attachments")

	return nil
}

func (s *idxStore) Announcement(ctx context.Context, id ...string) ([]model.Announcements, error) {
	ctx, span := s.tracer.Start(ctx, "store.idxStore.Announcement")
	defer span.End()

	logger := s.logger.With(slog.String("tag", "store.idxStore.Announcement"))

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

func (s *idxStore) Attachment(
	ctx context.Context,
	id ...string,
) (model.Attachments, error) {
	ctx, span := s.tracer.Start(ctx, "store.idxStore.Attachment")
	defer span.End()

	logger := s.logger.With(slog.String("tag", "store.idxStore.Attachment"))

	if len(id) == 0 {
		logger.DebugContext(ctx, "storagepath empty returning")
		span.AddEvent("storagepath empty returning")
		return model.Attachments{}, nil
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Attachments.SELECT(Attachments.AllColumns).
		WHERE(Attachments.ID.EQ(ANY(StringArray(id...)))).
		LIMIT(1)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "getting attachment")
	span.AddEvent("getting attachment")
	var attachment model.Attachments
	if err := stmt.QueryContext(ctx, s.db, &attachment); err != nil {
		err = fmt.Errorf("get attachment: %w", err)
		if errors.Is(err, qrm.ErrNoRows) {
			logger.WarnContext(ctx, "attachment not found", slog.Any("error", err))
			return attachment, err
		}
		logger.ErrorContext(ctx, "get attachment", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return attachment, err
	}
	logger.InfoContext(ctx, "got attachment")
	span.AddEvent("got attachment")

	return attachment, nil
}

func (s *idxStore) UnprocessedAttachments(ctx context.Context) ([]model.Attachments, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"store.idxStore.GetUnprocessedAttachments",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "store.idxStore.GetUnprocessedAttachments"))

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := SELECT(Attachments.AllColumns).
		FROM(Attachments).
		WHERE(
			Attachments.IsDownloaded.IS_FALSE().
				AND(Attachments.IsProcessing.IS_FALSE()).
				AND(Attachments.Error.EQ(String(""))),
		)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "getting unprocessed attachments")
	span.AddEvent("getting unprocessed attachments")
	var attachments []model.Attachments
	if err := stmt.QueryContext(ctx, s.db, &attachments); err != nil {
		err = fmt.Errorf("getting unprocessed attachments: %w", err)
		logger.ErrorContext(ctx, "getting unprocessed attachments", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return nil, err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.InfoContext(ctx, "got unprocessed attachments", slog.Int("count", len(attachments)))
	span.AddEvent("got unprocessed attachments")

	return attachments, nil
}

func (s *idxStore) ClaimAttachments(ctx context.Context, id ...string) error {
	ctx, span := s.tracer.Start(
		ctx,
		"store.idxStore.ClaimAttachments",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.idxStore.ClaimAttachments"),
		slog.Int("count", len(id)),
	)

	if len(id) == 0 {
		logger.DebugContext(ctx, "announcements empty returning")
		span.AddEvent("announcements empty returning")
		return nil
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Attachments.UPDATE(Attachments.IsProcessing).
		WHERE(
			Attachments.ID.EQ(ANY(StringArray(id...))).
				AND(Attachments.IsProcessing.IS_FALSE()).
				AND(Attachments.IsDownloaded.IS_FALSE()),
		).
		SET(Attachments.IsProcessing.SET(Bool(true))).
		RETURNING(Attachments.AllColumns)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "claiming attachments")
	span.AddEvent("claiming attachments")
	var claimAttachments []model.Attachments
	if err := stmt.QueryContext(ctx, s.db, &claimAttachments); err != nil {
		err = fmt.Errorf("claiming attachments: %w", err)
		logger.ErrorContext(ctx, "claiming attachments", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.Any("claimed_attachments", claimAttachments))
	}
	logger.InfoContext(ctx, "claimed attachments", slog.Int("claimed", len(claimAttachments)))
	span.AddEvent("claimed attachments")

	return nil
}

func (s *idxStore) UpdateAttachmentResult(ctx context.Context, attachment model.Attachments) error {
	ctx, span := s.tracer.Start(
		ctx,
		"store.idxStore.UpdateAttachmentResult",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "store.idxStore.UpdateAttachmentResult"),
		slog.String("id", attachment.ID.String()),
		slog.Bool("is_downloaded", attachment.IsDownloaded),
	)

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Attachments.UPDATE(Attachments.MutableColumns.Except(Attachments.ID)).
		WHERE(Attachments.ID.EQ(String(attachment.ID.String()))).
		SET(
			Attachments.IsDownloaded.SET(Bool(true)),
			Attachments.IsProcessing.SET(Bool(false)),
			Attachments.Checksum.SET(String(attachment.Checksum)),
			Attachments.StoragePath.SET(String(attachment.StoragePath)),
			Attachments.Error.SET(String(attachment.Error)),
			Attachments.UploadedAt.SET(TimestampzT(time.Now())),
		).RETURNING(Attachments.AllColumns)
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "updating attachment result")
	span.AddEvent("updating attachment result")
	var updated []model.Attachments
	if err := stmt.QueryContext(ctx, s.db, &updated); err != nil {
		err = fmt.Errorf("updating attachment result: %w", err)
		logger.ErrorContext(ctx, "updating attachment result", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return err
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = slogctx.Append(ctx, slog.Any("updated_attachments", updated))
	}
	logger.InfoContext(ctx, "updated attachment result")
	span.AddEvent("updated attachment result")

	return nil
}
