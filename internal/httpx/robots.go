package httpx

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"
)

// robotsTTL define por quanto tempo um robots.txt fica em cache.
const robotsTTL = 12 * time.Hour

type robotsRules struct {
	allow    []string
	disallow []string
	fetched  time.Time
}

// robotsCache baixa e guarda o robots.txt de cada host.
type robotsCache struct {
	c     *Client
	mu    sync.Mutex
	rules map[string]*robotsRules
	locks map[string]*sync.Mutex
}

func newRobotsCache(c *Client) *robotsCache {
	return &robotsCache{c: c, rules: map[string]*robotsRules{}, locks: map[string]*sync.Mutex{}}
}

func (rc *robotsCache) hostLock(key string) *sync.Mutex {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	l, ok := rc.locks[key]
	if !ok {
		l = &sync.Mutex{}
		rc.locks[key] = l
	}
	return l
}

func (rc *robotsCache) allowed(ctx context.Context, u *url.URL, ua string) (bool, error) {
	key := u.Scheme + "://" + u.Host
	l := rc.hostLock(key)
	l.Lock()
	defer l.Unlock()

	rc.mu.Lock()
	r := rc.rules[key]
	rc.mu.Unlock()
	if r == nil || time.Since(r.fetched) > robotsTTL {
		var err error
		r, err = rc.fetch(ctx, key, ua)
		if err != nil {
			return false, err
		}
		rc.mu.Lock()
		rc.rules[key] = r
		rc.mu.Unlock()
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return r.permits(path), nil
}

func (rc *robotsCache) fetch(ctx context.Context, base, ua string) (*robotsRules, error) {
	resp, err := rc.c.Do(ctx, Request{URL: base + "/robots.txt", NoRobot: true})
	if err != nil {
		var se *StatusError
		// 4xx: sem robots.txt, tudo permitido. 5xx persistente / rede: erro.
		if errors.As(err, &se) && se.Code >= 400 && se.Code < 500 {
			return &robotsRules{fetched: time.Now()}, nil
		}
		return nil, err
	}
	defer resp.Body.Close()
	r := parseRobots(io.LimitReader(resp.Body, 512<<10), ua)
	r.fetched = time.Now()
	return r, nil
}

// parseRobots aplica o grupo mais específico para o nosso User-Agent;
// se não houver, usa o grupo "*".
func parseRobots(rd io.Reader, ua string) *robotsRules {
	token := strings.ToLower(ua)
	if i := strings.IndexAny(token, "/ "); i >= 0 {
		token = token[:i]
	}
	type group struct {
		agents []string
		rules  robotsRules
	}
	var groups []*group
	var cur *group
	lastWasAgent := false
	sc := bufio.NewScanner(rd)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch k {
		case "user-agent":
			if cur == nil || !lastWasAgent {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(v))
			lastWasAgent = true
		case "allow", "disallow":
			lastWasAgent = false
			if cur == nil || v == "" {
				continue
			}
			if k == "allow" {
				cur.rules.allow = append(cur.rules.allow, v)
			} else {
				cur.rules.disallow = append(cur.rules.disallow, v)
			}
		default:
			lastWasAgent = false
		}
	}
	var star *group
	for _, g := range groups {
		for _, a := range g.agents {
			switch {
			case a == "":
			case a == "*":
				if star == nil {
					star = g
				}
			case token != "" && strings.Contains(token, a):
				return &g.rules
			}
		}
	}
	if star != nil {
		return &star.rules
	}
	return &robotsRules{}
}

// permits usa a regra de maior prefixo; empate favorece Allow.
func (r *robotsRules) permits(path string) bool {
	best, allowed := -1, true
	for _, p := range r.disallow {
		if n := matchLen(p, path); n > best {
			best, allowed = n, false
		}
	}
	for _, p := range r.allow {
		if n := matchLen(p, path); n >= best && n >= 0 {
			best, allowed = n, true
		}
	}
	return allowed
}

// matchLen devolve o tamanho do padrão se casar com path (suporta '*' e '$'),
// ou -1.
func matchLen(pattern, path string) int {
	anchored := strings.HasSuffix(pattern, "$")
	p := strings.TrimSuffix(pattern, "$")
	parts := strings.Split(p, "*")
	pos := 0
	for i, part := range parts {
		if i == 0 {
			if !strings.HasPrefix(path, part) {
				return -1
			}
			pos = len(part)
			continue
		}
		j := strings.Index(path[pos:], part)
		if j < 0 {
			return -1
		}
		pos += j + len(part)
	}
	if anchored && pos != len(path) && !strings.HasSuffix(pattern, "*$") {
		return -1
	}
	return len(pattern)
}
