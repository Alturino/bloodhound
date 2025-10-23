package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/Alturino/bloodhound/internal/client"
	"github.com/Alturino/bloodhound/internal/common/constants"
	"github.com/Alturino/bloodhound/internal/config"
	"github.com/Alturino/bloodhound/internal/db"
	"github.com/Alturino/bloodhound/internal/logging"
	"github.com/Alturino/bloodhound/internal/nats"
	"github.com/Alturino/bloodhound/internal/otel/otelutil"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/internal/response"
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
	pool := db.Get(ctx, cfg.Database)
	defer func() {
		logger.Debug().Msg("closing db")
		pool.Close()
		logger.Info().Msg("closed db")
	}()
	logger.Info().Msg("initialized db")

	logger.Debug().Msg("initializing nats")
	natsConn := nats.Get(ctx, cfg.Nats)
	defer func() {
		logger.Debug().Msg("closing nats")
		natsConn.Close()
		logger.Info().Msg("closed nats")
	}()
	logger.Info().Msg("initialized nats")

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

			var reply response.Reply
			lg := logger.With().
				Str("message_subject", msg.Subject()).
				Str("message_data", string(msg.Data())).
				Logger()
			if err := json.Unmarshal(msg.Data(), &reply); err != nil {
				err = fmt.Errorf("failed to unmarshal data with err: %w", err)
				lg.Error().Err(err).Msg(err.Error())
				if nakErr := msg.Nak(); nakErr != nil {
					nakErr = fmt.Errorf("failed to nak message with err: %w", nakErr)
					lg.Error().Err(nakErr).Msg(nakErr.Error())
					return
				}
				return
			}

			lg = lg.With().Any("reply", reply).Logger()
			ctx = lg.WithContext(ctx)
			if err := repo.DownloadAnnouncement(ctx, reply); err != nil {
				err = fmt.Errorf("failed to download announcement with err: %w", err)
				lg.Error().Err(err).Msg(err.Error())
				if nakErr := msg.Nak(); nakErr != nil {
					nakErr = fmt.Errorf("failed to nak message with err: %w", nakErr)
					lg.Error().Err(nakErr).Msg(nakErr.Error())
					return
				}
				return
			}
			lg.Info().Msg("successfully downloaded announcement")

			if err := msg.Ack(); err != nil {
				err = fmt.Errorf("failed to ack message with err: %w", err)
				lg.Error().Err(err).Msg(err.Error())
			}
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
