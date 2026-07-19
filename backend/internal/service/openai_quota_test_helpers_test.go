package service

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/imroc/req/v3"
)

type stubQuotaAccountRepo struct {
	AccountRepository
	accounts map[int64]*Account
}

func (r *stubQuotaAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	account, ok := r.accounts[id]
	if !ok {
		return nil, fmt.Errorf("account %d not found", id)
	}
	return account, nil
}

func (r *stubQuotaAccountRepo) UpdateCredentials(_ context.Context, id int64, credentials map[string]any) error {
	account, ok := r.accounts[id]
	if !ok {
		return fmt.Errorf("account %d not found", id)
	}
	account.Credentials = credentials
	return nil
}

type stubQuotaTokenCache struct {
	tokens map[string]string
}

func (c *stubQuotaTokenCache) GetAccessToken(_ context.Context, key string) (string, error) {
	if token, ok := c.tokens[key]; ok {
		return token, nil
	}
	return "", errors.New("token not found")
}

func (c *stubQuotaTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *stubQuotaTokenCache) DeleteAccessToken(context.Context, string) error { return nil }

func (c *stubQuotaTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *stubQuotaTokenCache) ReleaseRefreshLock(context.Context, string) error { return nil }

func newQuotaRedirectingFactory(server *httptest.Server) PrivacyClientFactory {
	targetURL, _ := url.Parse(server.URL)
	return func(string) (*req.Client, error) {
		client := req.C().WrapRoundTripFunc(func(roundTripper req.RoundTripper) req.RoundTripFunc {
			return func(request *req.Request) (*req.Response, error) {
				request.URL.Scheme = targetURL.Scheme
				request.URL.Host = targetURL.Host
				return roundTripper.RoundTrip(request)
			}
		})
		return client, nil
	}
}
