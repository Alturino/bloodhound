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
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
	. "github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/table"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type AttachmentStore interface {
	InsertAttachment(ctx context.Context, db qrm.DB, attachments ...model.Attachments) error
	Attachment(ctx context.Context, db qrm.DB, id ...uuid.UUID) (model.Attachments, error)
	UnprocessedAttachments(ctx context.Context) ([]model.Attachments, error)
	ClaimAttachments(ctx context.Context, id ...uuid.UUID) ([]model.Attachments, error)
	UpdateAttachmentResult(ctx context.Context, attachment *model.Attachments) error
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
		slog.Int(constants.AttachmentSize, len(attachments)),
	)

	if len(attachments) == 0 {
		logger.InfoContext(ctx, "no attachments to insert")
		span.AddEvent("no attachments to insert")
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
		logger = logger.With(slog.String(constants.SQLStatement, stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "inserting attachments")
	span.AddEvent("inserting attachments")
	if err := stmt.QueryContext(ctx, db, &attachments); err != nil {
		err = fmt.Errorf("inserting attachments: %v", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger = logger.With(slog.Int(constants.InsertedCount, len(attachments)))
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
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(slog.String(constants.SQLStatement, stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	var attachment model.Attachments
	if err := stmt.QueryContext(ctx, db, &attachment); err != nil {
		err = fmt.Errorf("get attachment: %v", err)
		if !errors.Is(err, qrm.ErrNoRows) {
			telemetry.RecordError(span, err)
		}
		return attachment, err
	}
	logger.InfoContext(ctx, "got attachment")
	span.AddEvent("got attachment")

	return attachment, nil
}

func (s *attachmentStore) UnprocessedAttachments(ctx context.Context) ([]model.Attachments, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"idx.attachmentStore.UnprocessedAttachments",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(slog.String("tag", "idx.attachmentStore.UnprocessedAttachments"))

	span.AddEvent("getting unprocessed attachments")

	logger.DebugContext(ctx, "preparing statement")
	span.AddEvent("preparing statement")
	stmt := SELECT(Attachments.AllColumns).
		FROM(Attachments).
		WHERE(
			Attachments.IsDownloaded.IS_FALSE().
				AND(Attachments.IsProcessing.IS_FALSE()).
				AND(Attachments.StoragePath.EQ(String("")).OR(Attachments.Checksum.EQ(String("")))).
				AND(Attachments.Error.NOT_LIKE(String("%status_code=4%"))),
		).ORDER_BY(Attachments.Date.ASC()).
		LIMIT(5000) // limiting to 5k postgres only supports 16bit(65535) parameters
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(slog.String(constants.SQLStatement, stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "getting unprocessed attachments")
	span.AddEvent("getting unprocessed attachments")
	var attachments []model.Attachments
	if err := stmt.QueryContext(ctx, s.db, &attachments); err != nil {
		err = fmt.Errorf("getting unprocessed attachments: %v", err)
		if !errors.Is(err, qrm.ErrNoRows) {
			telemetry.RecordError(span, err)
		}
		return nil, err
	}
	logger = logger.With(slog.Int(constants.Count, len(attachments)))
	logger.InfoContext(ctx, "got unprocessed attachments")
	span.AddEvent("got unprocessed attachments")

	return attachments, nil
}

func (s *attachmentStore) ClaimAttachments(
	ctx context.Context,
	id ...uuid.UUID,
) (attachments []model.Attachments, err error) {
	ctx, span := s.tracer.Start(
		ctx,
		"idx.attachmentStore.ClaimAttachments",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "idx.attachmentStore.ClaimAttachments"),
		slog.Int(constants.Count, len(id)),
	)

	if len(id) == 0 {
		err = errors.New("attachments id is empty")
		return
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
				AND(Attachments.IsDownloaded.IS_FALSE()).
				AND(Attachments.IsProcessing.IS_FALSE()).
				AND(Attachments.StoragePath.EQ(String("")).OR(Attachments.Checksum.EQ(String("")))).
				AND(Attachments.Error.NOT_LIKE(String("%status_code=4%"))),
		).
		SET(Attachments.IsProcessing.SET(Bool(true))).
		RETURNING(Attachments.AllColumns)
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(slog.String(constants.SQLStatement, stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "claiming attachments")
	span.AddEvent("claiming attachments")
	if err = stmt.QueryContext(ctx, s.db, &attachments); err != nil {
		err = fmt.Errorf("claiming attachments: %v", err)
		telemetry.RecordError(span, err)
		return
	}
	logger.InfoContext(ctx, "claimed attachments", slog.Int(constants.Claimed, len(attachments)))
	span.AddEvent("claimed attachments")

	return
}

func (s *attachmentStore) UpdateAttachmentResult(
	ctx context.Context,
	attachment *model.Attachments,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"idx.attachmentStore.UpdateAttachmentResult",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(),
	)
	defer span.End()

	logger := s.logger.With(
		slog.String("tag", "idx.attachmentStore.UpdateAttachmentResult"),
		slog.Any(constants.Attachment, attachment),
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
		).
		RETURNING(Attachments.AllColumns)
	if logger.Enabled(ctx, slog.LevelDebug) {
		logger = logger.With(slog.String(constants.SQLStatement, stmt.DebugSql()))
	}
	logger.DebugContext(ctx, "prepared statement")
	span.AddEvent("prepared statement")

	logger.DebugContext(ctx, "updating attachment result")
	span.AddEvent("updating attachment result")
	var updated model.Attachments
	if err := stmt.QueryContext(ctx, s.db, &updated); err != nil {
		err = fmt.Errorf("updating attachment result: %v", err)
		telemetry.RecordError(span, err)
		return err
	}
	logger.InfoContext(ctx, "updated attachment result")
	span.AddEvent("updated attachment result")

	return nil
}
