package main

import (
	"context"
	"sync"
	"time"
)

// loader runs a screen's archive.org work in the background when it doesn't finish in time to
// answer: the reply then says "Loading…" and asks the firmware to refresh, and a later request takes
// the result. An artist's years can take 9 s (the Grateful Dead's 18,000 recordings) against the
// firmware's 15 s deadline.
type loader struct {
	mu   sync.Mutex
	jobs map[string]*job
}

type job struct {
	done chan struct{}
	val  any
	err  error
	at   time.Time
}

// get answers key with f's result when it comes within wait; else ok is false and f goes on. A
// finished job is kept 30 s for the requests that follow; a failed one is dropped once reported, so
// Retry starts it again.
func (l *loader) get(key string, wait time.Duration, f func(context.Context) (any, error)) (val any, err error, ok bool) {
	l.mu.Lock()
	if l.jobs == nil {
		l.jobs = map[string]*job{}
	}
	j := l.jobs[key]
	if j != nil {
		select {
		case <-j.done:
			if j.err != nil || time.Since(j.at) > 30*time.Second {
				delete(l.jobs, key)
			}
			l.mu.Unlock()
			if j.err != nil || time.Since(j.at) <= 30*time.Second {
				return j.val, j.err, true
			}
			return l.get(key, wait, f)
		default:
		}
	} else {
		j = &job{done: make(chan struct{})}
		l.jobs[key] = j
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			v, e := f(ctx)
			l.mu.Lock()
			j.val, j.err, j.at = v, e, time.Now()
			l.mu.Unlock()
			close(j.done)
		}()
	}
	l.mu.Unlock()
	select {
	case <-j.done:
		l.mu.Lock()
		if j.err != nil {
			delete(l.jobs, key)
		}
		l.mu.Unlock()
		return j.val, j.err, true
	case <-time.After(wait):
		return nil, nil, false
	}
}

// running reports whether any job is still at work.
func (l *loader) running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, j := range l.jobs {
		select {
		case <-j.done:
		default:
			return true
		}
	}
	return false
}
