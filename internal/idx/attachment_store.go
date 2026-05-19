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
	"github.com/google/uuid"
	slogctx "github.com/veqryn/slog-context"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	. "github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/table"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type AttachmentStore interface {
	InsertAttachment(ctx context.Context, db qrm.DB, attachments ...model.Attachments) error
	Attachment(ctx context.Context, db qrm.DB, id ...uuid.UUID) (model.Attachments, error)
	UnprocessedAttachments(ctx context.Context, db qrm.DB) ([]model.Attachments, error)
	ClaimAttachments(ctx context.Context, db qrm.DB, id ...uuid.UUID) error
	UpdateAttachmentResult(ctx context.Context, db qrm.DB, attachment model.Attachments) error
}

type AttachmentStoreTask struct {
	AnnouncementID    string
	AnnouncementTitle string
	StockCode         string
	Date              time.Time
	Checksum          string
	Path              string
	Err               string
	Attachment        Attachment
}

func NewAttachmentStore(db *sql.DB, logger *slog.Logger, tracer trace.Tracer) AttachmentStore {
	if logger == nil {
		logger = slog.Default().With(slog.String("tag", "state.AttachmentStore"))
	}
	return &attachmentStore{db: db, logger: logger, tracer: tracer}
}

type attachmentStore struct {
	db     *sql.DB
	logger *slog.Logger
	tracer trace.Tracer
}

func (s *attachmentStore) InsertAttachment(
	ctx context.Context,
	db qrm.DB,
	attachments ...model.Attachments,
) error {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"idx.attachmentStore.InsertAttachment",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "idx.attachmentStore.InsertAttachment"),
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
	// if logger.Enabled(ctx, slog.LevelDebug) {
	// 	ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	// }
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.InfoContext(ctx, "inserting attachments")
	span.AddEvent("inserting attachments")
	if err := stmt.QueryContext(ctx, db, &attachments); err != nil {
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

func (s *attachmentStore) Attachment(
	ctx context.Context,
	db qrm.DB,
	id ...uuid.UUID,
) (model.Attachments, error) {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(ctx, "idx.attachmentStore.Attachment")
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.attachmentStore.Attachment"))

	if len(id) == 0 {
		logger.DebugContext(ctx, "storagepath empty returning")
		span.AddEvent("storagepath empty returning")
		return model.Attachments{}, nil
	}

	uuids := make([]StringExpression, len(id))
	for i, v := range id {
		uuids[i] = UUID(v)
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Attachments.SELECT(Attachments.AllColumns).
		WHERE(Attachments.ID.EQ(ANY(ARRAY(uuids...)))).
		LIMIT(1)
	// if logger.Enabled(ctx, slog.LevelDebug) {
	// 	ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	// }
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "getting attachment")
	span.AddEvent("getting attachment")
	var attachment model.Attachments
	if err := stmt.QueryContext(ctx, db, &attachment); err != nil {
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

func (s *attachmentStore) UnprocessedAttachments(
	ctx context.Context,
	db qrm.DB,
) ([]model.Attachments, error) {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"idx.attachmentStore.GetUnprocessedAttachments",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.attachmentStore.GetUnprocessedAttachments"))

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := SELECT(Attachments.AllColumns).
		FROM(Attachments).
		WHERE(
			Attachments.IsDownloaded.IS_FALSE().
				AND(Attachments.IsProcessing.IS_FALSE()).
				AND(Attachments.Error.EQ(String(""))),
		)
	// if logger.Enabled(ctx, slog.LevelDebug) {
	// 	ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	// }
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "getting unprocessed attachments")
	span.AddEvent("getting unprocessed attachments")
	var attachments []model.Attachments
	if err := stmt.QueryContext(ctx, db, &attachments); err != nil {
		err = fmt.Errorf("getting unprocessed attachments: %w", err)
		logger.ErrorContext(ctx, "getting unprocessed attachments", slog.Any("error", err))
		telemetry.RecordError(span, err)
		return nil, err
	}
	// if logger.Enabled(ctx, slog.LevelDebug) {
	// 	ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	// }
	logger.InfoContext(ctx, "got unprocessed attachments", slog.Int("count", len(attachments)))
	span.AddEvent("got unprocessed attachments")

	return attachments, nil
}

func (s *attachmentStore) ClaimAttachments(ctx context.Context, db qrm.DB, id ...uuid.UUID) error {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"idx.attachmentStore.ClaimAttachments",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "idx.attachmentStore.ClaimAttachments"),
		slog.Int("count", len(id)),
	)

	if len(id) == 0 {
		logger.DebugContext(ctx, "announcements empty returning")
		span.AddEvent("announcements empty returning")
		return nil
	}

	uuids := make([]StringExpression, len(id))
	for i, v := range id {
		uuids[i] = UUID(v)
	}

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Attachments.UPDATE(Attachments.IsProcessing).
		WHERE(
			Attachments.ID.EQ(ANY(ARRAY(uuids...))).
				AND(Attachments.IsProcessing.IS_FALSE()).
				AND(Attachments.IsDownloaded.IS_FALSE()),
		).
		SET(Attachments.IsProcessing.SET(Bool(true))).
		RETURNING(Attachments.AllColumns)
	// if logger.Enabled(ctx, slog.LevelDebug) {
	// 	ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	// }
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "claiming attachments")
	span.AddEvent("claiming attachments")
	var claimAttachments []model.Attachments
	if err := stmt.QueryContext(ctx, db, &claimAttachments); err != nil {
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

func (s *attachmentStore) UpdateAttachmentResult(
	ctx context.Context,
	db qrm.DB,
	attachment model.Attachments,
) error {
	if db == nil {
		db = s.db
	}
	ctx, span := s.tracer.Start(
		ctx,
		"idx.attachmentStore.UpdateAttachmentResult",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "idx.attachmentStore.UpdateAttachmentResult"),
		slog.String("id", attachment.ID.String()),
		slog.Bool("is_downloaded", attachment.IsDownloaded),
	)

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := Attachments.UPDATE(Attachments.MutableColumns.Except(Attachments.ID)).
		WHERE(Attachments.ID.EQ(UUID(attachment.ID))).
		SET(
			Attachments.IsDownloaded.SET(Bool(attachment.IsDownloaded)),
			Attachments.IsProcessing.SET(Bool(false)),
			Attachments.Checksum.SET(String(attachment.Checksum)),
			Attachments.StoragePath.SET(String(attachment.StoragePath)),
			Attachments.Error.SET(String(attachment.Error)),
			Attachments.UploadedAt.SET(TimestampzT(time.Now())),
		).RETURNING(Attachments.AllColumns)
	// if logger.Enabled(ctx, slog.LevelDebug) {
	// 	ctx = slogctx.Append(ctx, slog.String("sql_statement", stmt.DebugSql()))
	// }
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "updating attachment result")
	span.AddEvent("updating attachment result")
	var updated []model.Attachments
	if err := stmt.QueryContext(ctx, db, &updated); err != nil {
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
