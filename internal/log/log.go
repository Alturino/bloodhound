package log

import "log/slog"

type HttpClient struct {
	*slog.Logger
}

func NewHttpClient(logger *slog.Logger) HttpClient {
	if logger == nil {
		logger = slog.Default()
	}
	return HttpClient{Logger: logger.With(slog.String("tag", "httpclient"))}
}
