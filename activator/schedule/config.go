package schedule

import (
	"fmt"

	"github.com/mathspace/zipnap/config/duration"
	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

type Cron struct {
	cron.Schedule
}

// UnmarshalYAML implements custom unmarshalling for CronSchedule to handle
// cron expressions.
func (cs *Cron) UnmarshalYAML(n *yaml.Node) error {
	var expr string
	if err := n.Decode(&expr); err != nil {
		return fmt.Errorf("failed to decode cron expression: %w", err)
	}

	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}

	cs.Schedule = schedule
	return nil
}

// Config holds the configuration for the scheduler.
type Config struct {
	Cron Cron `yaml:"cron"`
	// KeepAwake specifies the duration the host is kept awake after it's woken up
	// by the scheduler.
	KeepAwake duration.Duration `yaml:"duration"`
}

func (c Config) Validate() error {
	if c.KeepAwake.Duration <= 0 {
		return fmt.Errorf("timeout must be positive, got %v", c.KeepAwake)
	}
	if c.Cron.Schedule == nil {
		return fmt.Errorf("cron schedule is not set")
	}
	return nil
}
