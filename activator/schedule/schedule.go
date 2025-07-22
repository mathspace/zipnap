// Package schedule implements a scheduled wake-up service using cron.
package schedule

import (
	"context"
	"log"

	"github.com/mathspace/zipnap/activator"
	"github.com/robfig/cron/v3"
)

type Schedule struct {
	cfg    *Config
	cb     activator.Callbacks
	logger *log.Logger
}

func New(cfg *Config, l *log.Logger) *Schedule {
	return &Schedule{cfg: cfg, logger: l}
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
			s.logger.Print("waking host")
			ctx, cancel := context.WithTimeout(ctx, s.cfg.KeepAwake.Duration)
			defer cancel()
			unlock, err := s.cb.WakeLock(ctx, true)
			if err != nil {
				return
			}
			defer unlock()
			<-ctx.Done()
			s.logger.Print("letting host sleep again")
		}()
	}))
	cr.Start()
	<-ctx.Done()
	cr.Stop()
	return nil
}
