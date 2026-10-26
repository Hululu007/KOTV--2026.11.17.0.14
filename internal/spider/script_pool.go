package spider

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// scriptPoolSize 同站 JS/Python 并发 worker 数（多用户打同一站时可并行）。
// 可用环境变量 KOTV_SCRIPT_POOL 覆盖（1–32）。
func scriptPoolSize() int {
	if v := strings.TrimSpace(os.Getenv("KOTV_SCRIPT_POOL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 32 {
			return n
		}
	}
	n := runtime.NumCPU()
	if n < 2 {
		n = 2
	}
	if n > 8 {
		n = 8
	}
	return n
}

// scriptSlotPool 是同站脚本槽位：空闲复用，满了等待，关闭后拒绝新调用。
type scriptSlotPool[T any] struct {
	mu      sync.Mutex
	slots   []T
	max     int
	waiters []chan struct{}
	busy    []bool
	closed  atomic.Bool
	spawn   func(index int) T
}

func (p *scriptSlotPool[T]) acquire() (T, int, error) {
	var zero T
	p.mu.Lock()
	for {
		if p.closed.Load() {
			p.mu.Unlock()
			return zero, -1, ErrScriptInterrupted
		}
		for i, s := range p.slots {
			if !p.busy[i] {
				p.busy[i] = true
				p.mu.Unlock()
				return s, i, nil
			}
		}
		if len(p.slots) < p.max {
			s := p.spawn(len(p.slots))
			p.slots = append(p.slots, s)
			p.busy = append(p.busy, true)
			idx := len(p.slots) - 1
			p.mu.Unlock()
			return s, idx, nil
		}
		ch := make(chan struct{})
		p.waiters = append(p.waiters, ch)
		p.mu.Unlock()
		<-ch
		p.mu.Lock()
	}
}

func (p *scriptSlotPool[T]) release(idx int) {
	p.mu.Lock()
	if idx >= 0 && idx < len(p.busy) {
		p.busy[idx] = false
	}
	if len(p.waiters) > 0 {
		ch := p.waiters[0]
		p.waiters = p.waiters[1:]
		p.mu.Unlock()
		ch <- struct{}{}
		return
	}
	p.mu.Unlock()
}

func (p *scriptSlotPool[T]) snapshot() []T {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]T(nil), p.slots...)
}

func borrowSlot[T any](p *scriptSlotPool[T]) (T, func(), error) {
	s, idx, err := p.acquire()
	if err != nil {
		var zero T
		return zero, func() {}, err
	}
	return s, func() { p.release(idx) }, nil
}

func withPooled[T any, R any](p *scriptSlotPool[T], fn func(T) (R, error)) (R, error) {
	var zero R
	s, release, err := borrowSlot(p)
	if err != nil {
		return zero, err
	}
	defer release()
	return fn(s)
}

func pooledMatchesClient[T interface{ activeClientID() string }](p *scriptSlotPool[T], clientID string) bool {
	if clientID == "" {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.slots {
		if s.activeClientID() == clientID {
			return true
		}
	}
	return false
}

func pooledInterrupt[T interface{ interrupt() }](p *scriptSlotPool[T]) {
	for _, s := range p.snapshot() {
		s.interrupt()
	}
}

func (p *scriptSlotPool[T]) shutdown(done func(T)) {
	p.mu.Lock()
	p.closed.Store(true)
	slots := append([]T(nil), p.slots...)
	p.slots = nil
	p.busy = nil
	waiters := p.waiters
	p.waiters = nil
	p.mu.Unlock()
	for _, ch := range waiters {
		close(ch)
	}
	for _, s := range slots {
		done(s)
	}
}
