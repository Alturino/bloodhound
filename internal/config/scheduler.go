package config

import "time"

type Scheduler struct {
	Interval       time.Duration `mapstructure:"interval"        json:"interval"`
	CronExpr       string        `mapstructure:"cron_expr"       json:"cron_expr"`       // e.g., "*/15 * * * *"
	HistoricalCron string        `mapstructure:"historical_cron" json:"historical_cron"` // "0 18 * * *" (6pm daily)
}
