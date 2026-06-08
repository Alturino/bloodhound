package httpclient

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/imroc/req/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/config"
	"github.com/alturino/bloodhound/internal/constants"
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
		SetCommonRetryBackoffInterval(1*time.Second, 5*time.Second)
		// WrapRoundTripFunc(traceReq(t)).
		// OnAfterResponse(errWrapper()).
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
		if !resp.IsSuccessState() {
			resp.Err = fmt.Errorf(
				"unexpected status_code=%d, url=%s, raw_content=%s",
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
			ctx := req.Context()
			ctx, span := tracer.Start(
				ctx,
				"httpclient",
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					attribute.String(constants.HTTPURL, req.URL.String()),
					attribute.String(constants.HTTPMethod, req.Method),
					attribute.String(constants.HTTPReqHeader, req.HeaderToString()),
					attribute.String(constants.HTTPReqBody, string(req.Body)),
					attribute.Int(constants.HTTPReqRetry, req.RetryAttempt),
					attribute.String(constants.HTTPReqStartTime, req.StartTime.String()),
				),
			)
			defer span.End()

			ctx = baggage.ContextWithoutBaggage(ctx)
			req = req.SetContext(ctx)

			resp, err = rt.RoundTrip(req)
			if err != nil {
				telemetry.RecordError(span, err)
				return nil, err
			}
			if resp.Response != nil {
				endTime := resp.Request.StartTime.Add(resp.TotalTime())
				span.SetAttributes(
					attribute.String(constants.HTTPRespDuration, resp.TotalTime().String()),
					attribute.Int(constants.HTTPRespStatus, resp.GetStatusCode()),
					attribute.String(constants.HTTPRespHeaders, resp.HeaderToString()),
					attribute.String(constants.HTTPRespEndTime, endTime.String()),
					attribute.String(constants.HTTPRespBody, resp.String()),
				)
			}
			return resp, err
		})
	})
}
