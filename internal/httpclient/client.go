package httpclient

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/telemetry"
)

func NewClient(config *config.Config, t trace.Tracer) *req.Client {
	// jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	client := req.C().ImpersonateChrome().
		EnableAutoDecompress().
		EnableAutoReadResponse().
		SetLogger(req.NewLoggerFromStandardLogger(slog.NewLogLogger(slog.Default().Handler(), config.App.LogLevelVar.Level()))).
		SetTimeout(30*time.Second).
		SetCommonRetryCount(3).
		SetCommonRetryBackoffInterval(1*time.Second, 5*time.Minute).
		WrapRoundTripFunc(traceReq(t)).
		OnAfterResponse(errWrapper())
	// AddCommonRetryCondition(middleware.ShouldGetCookie()).
	// SetCommonRetryHook(middleware.GetCookie(ctx)).
	// SetOutputDirectory(common.BloodhoundDir).
	// SetCookieJar(jar)
	if config.App.Environment != "production" {
		client = client.EnableDebugLog().EnableTraceAll().EnableDumpEachRequest()
	}
	return client
}

func errWrapper() req.ResponseMiddleware {
	return req.ResponseMiddleware(func(client *req.Client, resp *req.Response) error {
		if resp.Err != nil {
			if dump := resp.Dump(); dump != "" {
				resp.Err = fmt.Errorf("%v: raw_content=%s", resp.Err, dump)
			}
			return nil
		}
		if resp.IsErrorState() {
			resp.Err = fmt.Errorf(
				"unexpected status_code=%d, url=%s, http_dump=%s",
				resp.StatusCode,
				resp.Request.URL.String(),
				resp.Dump(),
			)
		}
		return nil
	})
}

func traceReq(tracer trace.Tracer) req.RoundTripWrapperFunc {
	return req.RoundTripWrapperFunc(func(rt req.RoundTripper) req.RoundTripFunc {
		return req.RoundTripFunc(func(req *req.Request) (resp *req.Response, err error) {
			_, span := tracer.Start(
				req.Context(),
				"httpclient",
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					attribute.String("http.url", req.URL.String()),
					attribute.String("http.method", req.Method),
					attribute.String("http.req.header", req.HeaderToString()),
					attribute.String("http.req.body", string(req.Body)),
					attribute.Int("http.req.retry_attempt", req.RetryAttempt),
					attribute.String("http.req.start_time", req.StartTime.String()),
				),
			)
			defer span.End()

			resp, err = rt.RoundTrip(req)
			if err != nil {
				err = fmt.Errorf("round trip: %v", err)
				telemetry.RecordError(span, err)
				return nil, err
			}
			if resp.Response != nil {
				endTime := resp.Request.StartTime.Add(resp.TotalTime())
				span.SetAttributes(
					attribute.String("http.resp.duration", resp.TotalTime().String()),
					attribute.Int("http.resp.status_code", resp.GetStatusCode()),
					attribute.String("http.resp.headers", resp.HeaderToString()),
					attribute.String("http.resp.end_time", endTime.String()),
					attribute.String("http.resp.body", resp.String()),
				)
			}
			return resp, err
		})
	})
}
