package scheduler

import (
	"fmt"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

type CronSchedule struct {
	cron.Schedule
}

// UnmarshalYAML implements custom unmarshalling for CronSchedule to handle
// cron expressions.
func (cs *CronSchedule) UnmarshalYAML(n *yaml.Node) error {
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
