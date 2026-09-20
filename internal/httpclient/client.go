package httpclient

import (
	"log/slog"
	"time"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
)

func NewClient(config *config.Config, t trace.Tracer) *req.Client {
	client := req.C().ImpersonateChrome().
		EnableAutoDecompress().
		EnableAutoReadResponse().
		SetLogger(req.NewLoggerFromStandardLogger(slog.NewLogLogger(slog.Default().Handler(), config.App.LogLevelVar.Level()))).
		SetTimeout(30*time.Second).
		SetCommonRetryCount(3).
		SetCommonRetryBackoffInterval(1*time.Second, 5*time.Second)
	if config.App.Environment != "production" {
		client = client.EnableDebugLog().EnableTraceAll().EnableDumpEachRequest()
	}
	return client
}
