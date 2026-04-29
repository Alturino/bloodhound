package httpclient

import (
	"log/slog"
	"time"

	"github.com/imroc/req/v3"

	"github.com/alturino/bloodhound/config"
)

func NewClient(config *config.Config) *req.Client {
	// jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	client := req.C().ImpersonateChrome().
		EnableAutoDecompress().
		SetLogger(req.NewLoggerFromStandardLogger(slog.NewLogLogger(slog.Default().Handler(), config.App.LogLevelVar.Level()))).
		SetTimeout(30*time.Second).
		SetCommonRetryCount(3).
		SetCommonRetryBackoffInterval(1*time.Second, 5*time.Second)
		// AddCommonRetryCondition(middleware.ShouldGetCookie()).
		// SetCommonRetryHook(middleware.GetCookie(ctx)).
		// SetOutputDirectory(common.BloodhoundDir).
		// SetCookieJar(jar)
	// if config.App.Environment != "production" {
	// 	client = client.DevMode().EnableDumpAllWithoutResponseBody().EnableDumpAllAsync()
	// }
	return client
}
