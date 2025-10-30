package downloader

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/minio/minio-go/v7"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/Alturino/bloodhound/internal/client"
	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
	"github.com/Alturino/bloodhound/internal/db"
	"github.com/Alturino/bloodhound/internal/db/.gen/postgres/public/model"
	. "github.com/Alturino/bloodhound/internal/db/.gen/postgres/public/table"
	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/logging"
	"github.com/Alturino/bloodhound/internal/nats"
	"github.com/Alturino/bloodhound/internal/nfs"
	inOtel "github.com/Alturino/bloodhound/internal/otel"
	"github.com/Alturino/bloodhound/internal/otel/otelutil"
	"github.com/Alturino/bloodhound/internal/repository"
)

func StartDownloader(ctx context.Context) {
	logger := log.Logger
	configPath := filepath.Join(".", "downloader.yaml")

	cfg, err := config.Get(ctx, configPath)
	if err != nil {
		err = fmt.Errorf("failed to get config with error: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}

	shutdownFuncs, err := inOtel.InitOtelSdk(ctx, "downloader", cfg.Otel)
	if err != nil {
		err = fmt.Errorf("failed to initialize otel with error: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}
	defer func() {
		logger.Debug().Msg("shutting down otel")
		inOtel.ShutdownOtel(ctx, shutdownFuncs)
		logger.Info().Msg("shut down otel")
	}()

	logger = logging.Get().With().
		Str(constants.KEY_TAG, "downloader StartDownloader").
		Str(constants.KEY_APP, "downloader").
		Any(constants.KEY_CONFIG, cfg).
		Logger()
	ctx = logger.WithContext(ctx)

	logger.Debug().Msg("initializing db")
	pool, err := db.Get(ctx, cfg.Database)
	if err != nil {
		err = fmt.Errorf("failed get pgxpool with error: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}
	logger.Info().Msg("initialized db")

	sqlDB := db.GetSQL(pool)
	defer func() {
		logger.Debug().Msg("closing sql db")
		if err := sqlDB.Close(); err != nil {
			err = fmt.Errorf("failed to close sql db with error: %w", err)
			logger.Error().Err(err).Msg(err.Error())
			return
		}
		logger.Info().Msg("closed sql db")
	}()

	logger.Debug().Msg("initializing nats")
	natsConn, err := nats.Get(ctx, cfg.Nats)
	if err != nil {
		err = fmt.Errorf("failed to get nats with error: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}
	defer func() {
		logger.Debug().Msg("closing nats")
		natsConn.Close()
		logger.Info().Msg("closed nats")
	}()
	logger.Info().Msg("initialized nats")

	logger.Debug().Msg("initializing minio client client")
	minioClient, err := nfs.Get(ctx, cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg(err.Error())
	}
	logger.Info().Msg("initialized minio client")
	if err := minioClient.MakeBucket(ctx, "bloodhound", minio.MakeBucketOptions{}); err != nil {
		err = fmt.Errorf("error creating bucket: %w", err)
		logger.Warn().Err(err).Msg(err.Error())
	}

	js, _ := nats.GetJetStream(ctx, natsConn)
	consumer, err := js.CreateOrUpdateConsumer(
		ctx,
		constants.STREAM_DOWNLADER,
		jetstream.ConsumerConfig{
			Durable:       "consumer:payment",
			FilterSubject: constants.EVENT_DOWNLOAD,
			DeliverPolicy: jetstream.DeliverAllPolicy,
		},
	)
	if err != nil {
		err = fmt.Errorf("failed to create or update consumer with err: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}

	httpClient := client.NewHTTPClient(ctx)
	repo := repository.NewHTTPRepository(httpClient)

	_, err = consumer.Consume(
		handleMessage(ctx, sqlDB, repo, minioClient),
		jetstream.ConsumeErrHandler(func(consumeCtx jetstream.ConsumeContext, err error) {
			err = fmt.Errorf("failed to consume with err: %w", err)
			logger.Error().Err(err).Msg(err.Error())
		}),
	)
	if err != nil {
		err = fmt.Errorf("failed to create consume handler with err: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}

	logger.Info().Msg("downloader is running, waiting for shutdown signal")
	<-ctx.Done()
	logger.Info().Msg("received shutdown signal, exiting downloader")
}

func handleMessage(
	ctx context.Context,
	sqlDB *sql.DB,
	repo *repository.HTTPRepository,
	minioClient *minio.Client,
) jetstream.MessageHandler {
	return func(msg jetstream.Msg) {
		headerCarrier := propagation.HeaderCarrier(msg.Headers())
		ctx := otel.GetTextMapPropagator().Extract(ctx, headerCarrier)

		ctx, span := otelutil.Tracer.Start(ctx, "downloader")
		defer span.End()

		logger := zerolog.Ctx(ctx).With().
			Str("message_subject", msg.Subject()).
			Str("message_data", string(msg.Data())).
			Logger()
		ctx = logger.WithContext(ctx)

		var err error
		defer func() {
			if err != nil {
				err = fmt.Errorf("failed to process message with err: %w", err)
				if nakErr := msg.Nak(); nakErr != nil {
					err = fmt.Errorf("%w failed to nak message with err: %w", err, nakErr)
					logger.Error().Err(err).Msg(err.Error())
				}
				return
			}
			if ackErr := msg.Ack(); ackErr != nil {
				err = fmt.Errorf("failed to ack message with err: %w", ackErr)
				logger.Error().Err(err).Msg(err.Error())
			}
		}()

		err = func() error {
			tx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{})
			if err != nil {
				return fmt.Errorf("failed to begin transaction with error: %w", err)
			}
			defer func() {
				if err != nil {
					err = fmt.Errorf("rolling back because of error: %w", err)
					if rbErr := tx.Rollback(); rbErr != nil {
						rbErr = fmt.Errorf("failed rolling back transaction with error: %w", rbErr)
						err = errors.Join(err, rbErr)
						if !errors.Is(err, sql.ErrTxDone) {
							logger.Error().Err(rbErr).Msg(rbErr.Error())
							otelutil.RecordError(rbErr, span)
							return
						}
						logger.Warn().Err(rbErr).Msg(rbErr.Error())
						span.AddEvent(rbErr.Error())
					}
					return
				}
				if err := tx.Commit(); err != nil {
					err = fmt.Errorf("failed committing transaction with error: %w", err)
					if !errors.Is(err, sql.ErrTxDone) {
						logger.Error().Err(err).Msg(err.Error())
						otelutil.RecordError(err, span)
						return
					}
					logger.Warn().Err(err).Msg(err.Error())
					return
				}
				logger.Info().Msg("successfully committed transaction")
			}()

			var data jobs.DownloadAttachmentArgs
			if err = json.Unmarshal(msg.Data(), &data); err != nil {
				return fmt.Errorf("failed to unmarshal data with err: %w", err)
			}

			var company model.Companies
			err = Companies.SELECT(Companies.AllColumns).
				WHERE(Companies.Ticker.EQ(String(data.Announcement.Ticker))).
				QueryContext(ctx, tx, &company)
			if err != nil {
				if errors.Is(err, qrm.ErrNoRows) {
					return fmt.Errorf("company not found with err: %w", err)
				}
				return fmt.Errorf("failed to get company with err: %w", err)
			}

			logger = logger.With().Any("data", data).Logger()
			ctx = logger.WithContext(ctx)
			downloadedFile, err := repo.DownloadFile(ctx, data)
			if err != nil {
				return fmt.Errorf("failed to download announcement with err: %w", err)
			}
			logger.Info().Msg("successfully downloaded announcement")

			logger.Debug().Msg("uploading file to minio")
			info, err := minioClient.FPutObject(
				ctx,
				"bloodhound",
				filepath.Base(downloadedFile.Name()),
				downloadedFile.Name(),
				minio.PutObjectOptions{
					ContentType: "application/pdf",
					UserTags: map[string]string{
						"emiten":       data.Announcement.Ticker,
						"sector":       company.Sector.String(),
						"subsector":    company.SubSector.String(),
						"industry":     company.Industry.String(),
						"sub_industry": company.SubIndustry.String(),
					},
				},
			)
			if err != nil {
				err = fmt.Errorf("failed to download announcement with err: %w", err)
				return err
			}
			logger = logger.With().
				Str("saved_path", info.Location).
				Str("version_id", info.VersionID).
				Str("checksum", info.ChecksumSHA256).
				Logger()
			logger.Info().Msg("successfully uploaded file to minio")

			logger.Debug().Msg("getting announcement from db")
			var announcement model.Announcements
			err = Announcements.SELECT(Announcements.AllColumns).
				WHERE(Announcements.Name.EQ(String(data.Announcement.Title))).
				QueryContext(ctx, tx, &announcement)
			if err != nil {
				if errors.Is(err, qrm.ErrNoRows) {
					return fmt.Errorf("company not found with error: %w", err)
				}
				return fmt.Errorf("failed to get announcement with error: %w", err)
			}
			logger.Debug().Msg("got announcement from db")

			logger.Debug().Msg("creating attachment")
			attachment := model.Attachments{
				AnnouncementID: announcement.ID,
				Name:           data.Attachment.Filename,
				Path:           info.Location,
				Checksum:       info.ChecksumSHA256,
				SourceURL:      data.Attachment.DownloadURL,
				Type:           model.AttachmentType_Others,
				PublishedAt:    data.Announcement.Date.Time,
				CreatedAt:      info.LastModified,
			}
			err = Attachments.INSERT(Attachments.EXCLUDED.ID).
				MODEL(attachment).
				RETURNING(Attachments.AllColumns).
				QueryContext(ctx, tx, &attachment)
			if err != nil {
				return fmt.Errorf("attachment is not inserted with error: %w", err)
			}
			logger.Debug().Msg("created attachment")
			return nil
		}()
	}
}
