package limiter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/shivang-16/orbit.api/internal/config"
)

type Error struct {
	RetryAfter time.Duration
	Limit      int
	Remaining  int
	Reset      time.Time
	Scope      string
}

func (e *Error) Error() string {
	return fmt.Sprintf("rate limit exceeded (%s)", e.Scope)
}

type Reservation struct {
	Release   func()
	Limit     int
	Remaining int
	Reset     time.Time
}

type Limiter interface {
	Allow(orgID, userID string, playground bool) (*Reservation, error)
}

type Memory struct {
	limits config.RateLimits
	mu     sync.Mutex
	orgs   map[string]*orgState
}

type orgState struct {
	mu         sync.Mutex
	rpm        tokenBucket
	inflight   int
	max        int
	playground map[string]*tokenBucket
}

type tokenBucket struct {
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

type ctxKey int

const playgroundCtxKey ctxKey = 1

func WithPlayground(ctx context.Context) context.Context {
	return context.WithValue(ctx, playgroundCtxKey, true)
}

func IsPlayground(ctx context.Context) bool {
	v, _ := ctx.Value(playgroundCtxKey).(bool)
	return v
}

func NewMemory(limits config.RateLimits) *Memory {
	if limits.Organization.Concurrent < 1 {
		limits.Organization.Concurrent = 1
	}
	return &Memory{limits: limits, orgs: map[string]*orgState{}}
}

func (m *Memory) Allow(orgID, userID string, playground bool) (*Reservation, error) {
	if m == nil || orgID == "" {
		return &Reservation{Release: func() {}}, nil
	}

	state := m.state(orgID)
	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	orgLimit := m.limits.Organization.RequestsPerMinute

	if playground && userID != "" {
		pg := state.playgroundBucket(userID, m.limits.Playground)
		if ok, retry, remaining := pg.available(now); !ok {
			return nil, rateError("playground", m.limits.Playground.RequestsPerMinute, remaining, retry, now)
		}
	}
	if ok, retry, remaining := state.rpm.available(now); !ok {
		return nil, rateError("organization", orgLimit, remaining, retry, now)
	}
	if state.inflight >= state.max {
		return nil, rateError("concurrency", orgLimit, int(state.rpm.tokens), time.Second, now)
	}

	if playground && userID != "" {
		state.playgroundBucket(userID, m.limits.Playground).take()
	}
	state.rpm.take()
	state.inflight++
	return m.reservation(state, orgLimit, int(state.rpm.tokens), now), nil
}

func rateError(scope string, limit, remaining int, retry time.Duration, now time.Time) *Error {
	if retry < time.Second {
		retry = time.Second
	}
	return &Error{
		RetryAfter: retry,
		Limit:      limit,
		Remaining:  remaining,
		Reset:      now.Add(retry),
		Scope:      scope,
	}
}

func (m *Memory) reservation(state *orgState, limit, remaining int, now time.Time) *Reservation {
	var once sync.Once
	return &Reservation{
		Limit:     limit,
		Remaining: remaining,
		Reset:     now.Add(time.Minute),
		Release: func() {
			once.Do(func() {
				state.mu.Lock()
				if state.inflight > 0 {
					state.inflight--
				}
				state.mu.Unlock()
			})
		},
	}
}

func (m *Memory) state(orgID string) *orgState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.orgs[orgID]
	if !ok {
		rpm := newBucket(m.limits.Organization.RequestsPerMinute, m.limits.Organization.Burst)
		state = &orgState{
			rpm:        rpm,
			max:        m.limits.Organization.Concurrent,
			playground: map[string]*tokenBucket{},
		}
		m.orgs[orgID] = state
	}
	return state
}

func (s *orgState) playgroundBucket(userID string, window config.RateLimitWindow) *tokenBucket {
	if b, ok := s.playground[userID]; ok {
		return b
	}
	b := newBucket(window.RequestsPerMinute, window.Burst)
	s.playground[userID] = &b
	return s.playground[userID]
}

func newBucket(rpm, burst int) tokenBucket {
	if burst < 1 {
		burst = 1
	}
	if rpm < 1 {
		rpm = 1
	}
	return tokenBucket{
		rate:   float64(rpm) / 60.0,
		burst:  float64(burst),
		tokens: float64(burst),
		last:   time.Now(),
	}
}

func (b *tokenBucket) refill(now time.Time) {
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(b.burst, b.tokens+elapsed*b.rate)
		b.last = now
	}
}

func (b *tokenBucket) available(now time.Time) (bool, time.Duration, int) {
	b.refill(now)
	if b.tokens >= 1 {
		return true, 0, int(b.tokens) - 1
	}
	wait := (1 - b.tokens) / b.rate
	if wait < 0 {
		wait = 0
	}
	return false, time.Duration(wait * float64(time.Second)), 0
}

func (b *tokenBucket) take() {
	if b.tokens >= 1 {
		b.tokens -= 1
	}
}

func SetHeaders(w http.ResponseWriter, limit, remaining int, reset time.Time, retryAfter time.Duration) {
	if w == nil {
		return
	}
	h := w.Header()
	h.Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
	h.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	h.Set("X-RateLimit-Reset", fmt.Sprintf("%d", reset.Unix()))
	h.Set("x-ratelimit-limit-requests", fmt.Sprintf("%d", limit))
	h.Set("x-ratelimit-remaining-requests", fmt.Sprintf("%d", remaining))
	h.Set("x-ratelimit-reset-requests", fmt.Sprintf("%d", reset.Unix()))
	if retryAfter > 0 {
		sec := int(math.Ceil(retryAfter.Seconds()))
		if sec < 1 {
			sec = 1
		}
		h.Set("Retry-After", fmt.Sprintf("%d", sec))
	}
}

func SetHeadersFromError(w http.ResponseWriter, err error) bool {
	var rl *Error
	if !errors.As(err, &rl) {
		return false
	}
	SetHeaders(w, rl.Limit, rl.Remaining, rl.Reset, rl.RetryAfter)
	return true
}
