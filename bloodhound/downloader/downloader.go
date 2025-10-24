package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/minio/minio-go/v7"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/Alturino/bloodhound/internal/client"
	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
	"github.com/Alturino/bloodhound/internal/db"
	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/logging"
	"github.com/Alturino/bloodhound/internal/nats"
	"github.com/Alturino/bloodhound/internal/nfs"
	"github.com/Alturino/bloodhound/internal/otel/otelutil"
	"github.com/Alturino/bloodhound/internal/repository"
)

func StartDownloader(ctx context.Context) {
	configPath := filepath.Join(".", "downloader.yaml")

	cfg := config.Get(ctx, configPath)

	logger := logging.Get().With().
		Str(constants.KEY_TAG, "downloader StartDownloader").
		Str(constants.KEY_APP, "downloader").
		Any(constants.KEY_CONFIG, cfg).
		Logger()

	logger.Debug().Msg("initializing db")
	ctx = logger.WithContext(ctx)
	pool := db.Get(ctx, cfg.Database)
	defer func() {
		logger.Debug().Msg("closing db")
		pool.Close()
		logger.Info().Msg("closed db")
	}()
	logger.Info().Msg("initialized db")

	logger.Debug().Msg("initializing nats")
	ctx = logger.WithContext(ctx)
	natsConn, err := nats.Get(ctx, cfg.Nats)
	if err != nil {
		logger.Fatal().Err(err).Msg(err.Error())
	}
	defer func() {
		logger.Debug().Msg("closing nats")
		natsConn.Close()
		logger.Info().Msg("closed nats")
	}()
	logger.Info().Msg("initialized nats")

	minioClient, err := nfs.Get(ctx, cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg(err.Error())
	}

	js, _ := nats.GetJetStream(ctx, natsConn)
	consumer, err := js.CreateOrUpdateConsumer(
		ctx,
		constants.STREAM_DOWNLADER,
		jetstream.ConsumerConfig{
			Durable:       "consumer:payment",
			FilterSubject: constants.EVENT_DOWNLOAD,
			DeliverPolicy: jetstream.DeliverLastPolicy,
		},
	)
	if err != nil {
		err = fmt.Errorf("failed to create or update consumer with err: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}

	httpClient := client.NewHTTPClient(ctx)
	repo := repository.NewHTTPRepository(httpClient)

	_, err = consumer.Consume(
		func(msg jetstream.Msg) {
			headerCarrier := propagation.HeaderCarrier(msg.Headers())
			ctx := otel.GetTextMapPropagator().Extract(ctx, headerCarrier)

			ctx, span := otelutil.Tracer.Start(ctx, "downloader")
			defer span.End()

			lg := logger.With().
				Str("message_subject", msg.Subject()).
				Str("message_data", string(msg.Data())).
				Logger()

			var err error
			defer func() {
				if err != nil {
					err = fmt.Errorf("failed to process message with err: %w", err)
					if nakErr := msg.Nak(); nakErr != nil {
						err = fmt.Errorf("failed to nak message with err: %w %w", err, nakErr)
						lg.Error().Err(err).Msg(err.Error())
					}
					return
				}
				if ackErr := msg.Ack(); ackErr != nil {
					err = fmt.Errorf("failed to ack message with err: %w", ackErr)
					lg.Error().Err(err).Msg(err.Error())
				}
			}()

			var data jobs.DownloadAttachmentArgs
			if err = json.Unmarshal(msg.Data(), &data); err != nil {
				err = fmt.Errorf("failed to unmarshal data with err: %w", err)
				lg.Error().Err(err).Msg(err.Error())
				return
			}

			lg = lg.With().Any("reply", data).Logger()
			ctx = lg.WithContext(ctx)
			downloadedFile, err := repo.DownloadFile(ctx, data)
			if err != nil {
				err = fmt.Errorf("failed to download announcement with err: %w", err)
				lg.Error().Err(err).Msg(err.Error())
				return
			}
			lg.Info().Msg("successfully downloaded announcement")

			lg.Debug().Msg("uploading file to minio")
			info, err := minioClient.FPutObject(
				ctx,
				"bloodhound",
				filepath.Base(downloadedFile.Name()),
				downloadedFile.Name(),
				minio.PutObjectOptions{},
			)
			if err != nil {
				err = fmt.Errorf("failed to download announcement with err: %w", err)
				lg.Error().Err(err).Msg(err.Error())
				return
			}
			lg.Info().Msg("successfully uploaded file to minio")
			_ = info
		},
		jetstream.ConsumeErrHandler(func(consumeCtx jetstream.ConsumeContext, err error) {
			err = fmt.Errorf("failed to consume with err: %w", err)
			logger.Error().Err(err).Msg(err.Error())
		}),
	)
	if err != nil {
		err = fmt.Errorf("failed to create consume handler with err: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}

	<-ctx.Done()
	logger.Info().Msg("received shutdown signal, exiting downloader")
}
