// Package httpx fornece o cliente HTTP usado pelos adaptadores: User-Agent
// identificável, timeout, retry com backoff, concorrência limitada por host e
// respeito a robots.txt.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// ErrDisallowed indica que o robots.txt do host proíbe o caminho.
var ErrDisallowed = errors.New("bloqueado por robots.txt")

// StatusError é devolvido para respostas HTTP não-2xx.
type StatusError struct {
	Code int
	URL  string
}

func (e *StatusError) Error() string { return fmt.Sprintf("HTTP %d em %s", e.Code, e.URL) }

// Options configura o Client.
type Options struct {
	UserAgent      string
	RequestTimeout time.Duration
	PerHost        int
	Retries        int
	// BaseBackoff é o atraso da primeira nova tentativa (dobra a cada uma).
	BaseBackoff time.Duration
	// IgnoreRobots desliga a checagem de robots.txt (só para testes).
	IgnoreRobots bool
}

// Client é seguro para uso concorrente.
type Client struct {
	hc     *http.Client
	opts   Options
	mu     sync.Mutex
	hosts  map[string]chan struct{}
	robots *robotsCache
}

// New cria um Client.
func New(opts Options) *Client {
	if opts.PerHost < 1 {
		opts.PerHost = 1
	}
	if opts.BaseBackoff == 0 {
		opts.BaseBackoff = time.Second
	}
	c := &Client{
		hc: &http.Client{
			Timeout: opts.RequestTimeout,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConnsPerHost: opts.PerHost,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
				ForceAttemptHTTP2:   true,
			},
		},
		opts:  opts,
		hosts: make(map[string]chan struct{}),
	}
	c.robots = newRobotsCache(c)
	return c
}

func (c *Client) slot(host string) chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.hosts[host]
	if !ok {
		s = make(chan struct{}, c.opts.PerHost)
		c.hosts[host] = s
	}
	return s
}

// Request descreve uma requisição; Body é recriado a cada tentativa.
type Request struct {
	Method  string
	URL     string
	Header  http.Header
	Body    func() io.Reader
	NoRobot bool // pula robots.txt (usado só para buscar o próprio robots.txt)
}

// Get é um atalho para GET.
func (c *Client) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	return c.Do(ctx, Request{Method: http.MethodGet, URL: rawURL})
}

// Do executa a requisição com retry. Em caso de sucesso o chamador deve
// fechar resp.Body, o que também libera o slot do host.
func (c *Client) Do(ctx context.Context, r Request) (*http.Response, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, err
	}
	if !c.opts.IgnoreRobots && !r.NoRobot {
		ok, err := c.robots.allowed(ctx, u, c.opts.UserAgent)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrDisallowed, r.URL)
		}
	}
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	var lastErr error
	for attempt := 0; attempt <= c.opts.Retries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, c.backoff(attempt, lastErr)); err != nil {
				return nil, err
			}
		}
		resp, err := c.once(ctx, u.Host, r)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable(err) || ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) once(ctx context.Context, host string, r Request) (*http.Response, error) {
	var body io.Reader
	if r.Body != nil {
		body = r.Body()
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, r.URL, body)
	if err != nil {
		return nil, err
	}
	for k, vs := range r.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json, text/xml, text/html;q=0.9, */*;q=0.8")
	}

	s := c.slot(host)
	select {
	case s <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := sync.OnceFunc(func() { <-s })

	resp, err := c.hc.Do(req)
	if err != nil {
		release()
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		release()
		return nil, &retryAfterError{
			StatusError: &StatusError{Code: resp.StatusCode, URL: r.URL},
			after:       parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	resp.Body = &releaseBody{ReadCloser: resp.Body, release: release}
	return resp, nil
}

type releaseBody struct {
	io.ReadCloser
	release func()
}

func (b *releaseBody) Close() error {
	err := b.ReadCloser.Close()
	b.release()
	return err
}

type retryAfterError struct {
	*StatusError
	after time.Duration
}

func (e *retryAfterError) Unwrap() error { return e.StatusError }

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code == http.StatusTooManyRequests || se.Code >= 500
	}
	return true // erro de rede / timeout
}

func (c *Client) backoff(attempt int, lastErr error) time.Duration {
	var ra *retryAfterError
	if errors.As(lastErr, &ra) && ra.after > 0 {
		if ra.after > time.Minute {
			return time.Minute
		}
		return ra.after
	}
	d := c.opts.BaseBackoff << (attempt - 1)
	jitter := time.Duration(rand.Int63n(int64(d)/2 + 1))
	return d + jitter
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
