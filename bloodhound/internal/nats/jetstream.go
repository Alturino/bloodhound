package nats

import (
	"context"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/common/constants"
)

var (
	jsOnce    sync.Once
	jetStream jetstream.JetStream
	stream    jetstream.Stream
)

func GetJetStream(
	ctx context.Context,
	natsConn *nats.Conn,
) (jetstream.JetStream, jetstream.Stream) {
	logger := zerolog.Ctx(ctx).With().Str(constants.KEY_TAG, "nats GetJetStream").Logger()
	jsOnce.Do(func() {
		js, err := jetstream.New(natsConn)
		if err != nil {
			err = fmt.Errorf("failed to get jetstream with err: %w", err)
			logger.Fatal().Err(err).Msg(err.Error())
		}
		st, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:      constants.STREAM_DOWNLADER,
			Retention: jetstream.WorkQueuePolicy,
			Subjects:  []string{"events.>"},
		})
		if err != nil {
			err = fmt.Errorf("failed to create or update stream with err: %w", err)
			logger.Fatal().Err(err).Msg(err.Error())
		}
		stream = st
		jetStream = js
	})
	return jetStream, stream
}
