package domain

import (
	"fmt"
	"time"
)

func NewDeviation(trialID, kind, severity, reporter, cause string) DeviationCase {
	return DeviationCase{ID: fmt.Sprintf("DEV-%d", time.Now().UnixNano()), TrialID: trialID, Kind: kind, Severity: severity, DetectedAt: time.Now().UTC(), ReportedBy: reporter, RootCause: cause}
}
