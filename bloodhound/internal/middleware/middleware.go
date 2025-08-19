package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/imroc/req/v3"
	"github.com/rs/zerolog"

	"github.com/Alturino/bloodhound/internal/response"
)

func ShouldGetCookie() req.RetryConditionFunc {
	return func(resp *req.Response, err error) bool {
		return resp.StatusCode == http.StatusForbidden
	}
}

func GetCookie(ctx context.Context) req.RetryHookFunc {
	return func(resp *req.Response, err error) {
		logger := zerolog.Ctx(ctx).With().Logger()
		if err != nil {
			err = fmt.Errorf("failed to get data err: %w", err)
			logger.Error().Err(err).Msg(err.Error())
		}
		destinationURL := "https://idx.co.id/id"
		reqBody := map[string]any{
			"cmd":     "request.get",
			"url":     destinationURL,
			"timeout": time.Duration(time.Second * 60).Milliseconds(),
		}
		proxyURL := "http://127.0.0.1:8191/v1"
		logger.Debug().Msg("sending request through proxy")
		result, err := resp.Request.GetClient().R().
			SetBody(reqBody).
			Post(proxyURL)
		if err != nil {
			err = fmt.Errorf("failed to get cookie err: %w", err)
			logger.Error().Err(err).Msg(err.Error())
			return
		}
		logger.Debug().Msg("successfully sent through proxy")

		logger.Debug().Msg("unmarshal proxy response")
		var resBody response.ChallengeResponse
		if err = result.UnmarshalJson(&resBody); err != nil {
			err = fmt.Errorf("failed UnmarshalJson ChallengeResponse err: %w", err)
			logger.Error().Err(err).Msg(err.Error())
			return
		}
		logger = logger.With().
			Str("destination_url", destinationURL).
			Str("proxy_url", proxyURL).
			Any("challenge_response", resBody).
			Logger()

		cookies := []*http.Cookie{}
		for _, cookie := range resBody.Solution.Cookies {
			if cookie.Name == "__cf_bm" || cookie.Name == "cf_clearance" ||
				cookie.Name == "auth.strategy" ||
				cookie.Name == "_cfuvid" {
				cookies = append(cookies, &http.Cookie{
					Name:     cookie.Name,
					Value:    cookie.Value,
					Path:     cookie.Path,
					Domain:   cookie.Domain,
					Expires:  time.Unix(cookie.Expiry, 0),
					Secure:   cookie.Secure,
					HttpOnly: cookie.HTTPOnly,
					SameSite: func(sameSite string) http.SameSite {
						if sameSite == "Lax" {
							return http.SameSiteLaxMode
						}
						if sameSite == "None" {
							return http.SameSiteLaxMode
						}
						if sameSite == "Strict" {
							return http.SameSiteStrictMode
						}
						return http.SameSiteDefaultMode
					}(cookie.SameSite),
				})
			}
		}
		logger.Debug().Any("cookies", cookies).Send()

		c := resp.Request.GetClient()
		logger.Debug().Msg("set cookies to http client")
		resp.Request.SetClient(c.SetCommonCookies(cookies...))
		newHTTPClientCookie := resp.Request.GetClient().Clone().Cookies
		logger.Debug().Any("new_http_client_cookie", newHTTPClientCookie).Send()

		logger.Info().Msg("get cookie success")
	}
}
