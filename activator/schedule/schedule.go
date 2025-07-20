// Package schedule implements a scheduled wake-up service using cron.
package schedule

import (
	"context"

	"github.com/mathspace/zipnap/activator"
	"github.com/robfig/cron/v3"
)

type Schedule struct {
	cfg Config
	cb  activator.Callbacks
}

func New(cfg Config) *Schedule {
	return &Schedule{cfg: cfg}
}

func (s *Schedule) RegisterCallbacks(cb activator.Callbacks) {
	s.cb = cb
}

// Run starts the scheduler, which will wake the host according to the
// configured cron schedule.
func (s *Schedule) Run(ctx context.Context) error {
	cr := cron.New()
	cr.Schedule(s.cfg.Cron, cron.FuncJob(func() {
		go func() {
			ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout.Duration)
			defer cancel()
			unlock, err := s.cb.WakeLock(ctx, true)
			if err != nil {
				return
			}
			defer unlock()
			<-ctx.Done()
		}()
	}))
	cr.Start()
	<-ctx.Done()
	cr.Stop()
	return nil
}
