package core

import (
	"time"
)

func (r *Runtime) WaitSettings() (time.Duration, time.Duration) {
	return r.config.WaitTimeout, r.config.PollInterval
}
