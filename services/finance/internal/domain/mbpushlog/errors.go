package mbpushlog

import "errors"

// ErrPeriodRequired is returned when period is empty.
var ErrPeriodRequired = errors.New("mbpushlog: period is required")
