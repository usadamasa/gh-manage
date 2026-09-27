// Package github is the only place that talks to the GitHub REST API.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
)

const host = "github.com"

// Options configures a Client. Zero values select the defaults.
type Options struct {
	// Token is the API token. 空なら GH_TOKEN (GITHUB_TOKEN) → gh の設定 → `gh auth token` の順で探す｡
	Token string
	// Transport replaces the HTTP transport (tests inject a fake here).
	Transport http.RoundTripper
}

// Client reads and writes repository settings through the REST API.
type Client struct {
	rest *api.RESTClient
	rate *rateTransport
}

// New creates a Client.
func New(opts Options) (*Client, error) {
	token := opts.Token
	if token == "" {
		token, _ = auth.TokenForHost(host)
	}
	if token == "" {
		return nil, errors.New("GitHub の token が無い: GH_TOKEN を設定するか `gh auth login` する")
	}
	base := opts.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	rate := &rateTransport{base: base}
	rest, err := api.NewRESTClient(api.ClientOptions{
		Host:         host,
		AuthToken:    token,
		Transport:    rate,
		LogIgnoreEnv: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create REST client: %w", err)
	}
	return &Client{rest: rest, rate: rate}, nil
}

// RateLimitRemaining returns X-RateLimit-Remaining of the last response, or "" before any request.
func (c *Client) RateLimitRemaining() string {
	return c.rate.get()
}

// rateTransport records X-RateLimit-Remaining of every response.
type rateTransport struct {
	base      http.RoundTripper
	mu        sync.Mutex
	remaining string
}

func (t *rateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err //nolint:wrapcheck // transport のエラーはそのまま返す
	}
	if v := resp.Header.Get("X-RateLimit-Remaining"); v != "" {
		t.mu.Lock()
		t.remaining = v
		t.mu.Unlock()
	}
	return resp, nil
}

func (t *rateTransport) get() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.remaining
}

// getJSON decodes the response of GET path into out.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, out); err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	return nil
}

var nextLinkPattern = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// getPages calls page with the body of GET path and of every following page (Link: rel="next").
func (c *Client) getPages(ctx context.Context, path string, page func(body []byte) error) error {
	next := path
	for next != "" {
		resp, err := c.rest.RequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return fmt.Errorf("GET %s: %w", next, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("GET %s: read body: %w", next, err)
		}
		if err := page(body); err != nil {
			return fmt.Errorf("GET %s: %w", next, err)
		}
		next = ""
		if m := nextLinkPattern.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			next = m[1]
		}
	}
	return nil
}

// getList collects every element of a paginated JSON array.
func getList[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var all []T
	err := c.getPages(ctx, path, func(body []byte) error {
		var items []T
		if err := json.Unmarshal(body, &items); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		all = append(all, items...)
		return nil
	})
	return all, err
}
