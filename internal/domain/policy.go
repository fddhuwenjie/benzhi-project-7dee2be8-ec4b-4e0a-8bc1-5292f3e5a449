package domain

import (
	"errors"
	"strings"
)

func (p Protocol) ExposureTemperatureRange() (float64, float64) {
	return p.AgeTempC - 1, p.AgeTempC + 1
}
func (p Protocol) ExposureHumidityRange() (float64, float64) {
	return p.AgeHumidity - 5, p.AgeHumidity + 5
}

func (p Protocol) ExposureWithinTolerance(exposure Exposure) bool {
	minTemp, maxTemp := p.ExposureTemperatureRange()
	minHumidity, maxHumidity := p.ExposureHumidityRange()
	return exposure.TemperatureC >= minTemp && exposure.TemperatureC <= maxTemp && exposure.Humidity >= minHumidity && exposure.Humidity <= maxHumidity
}

func (d DeviationCase) ValidateForReport() error {
	if strings.TrimSpace(d.Kind) == "" {
		return errors.New("kind 必填")
	}
	if d.Severity != "LOW" && d.Severity != "MEDIUM" && d.Severity != "HIGH" {
		return errors.New("severity 必须为 LOW、MEDIUM 或 HIGH")
	}
	if strings.TrimSpace(d.RootCause) == "" {
		return errors.New("root_cause 必填")
	}
	if strings.TrimSpace(d.Disposition) == "" {
		return errors.New("disposition 必填")
	}
	return nil
}

func (t *VigorTrial) OpenDeviations() []DeviationCase {
	var result []DeviationCase
	for _, deviation := range t.Deviations {
		if deviation.Decision != "ACCEPTED" {
			result = append(result, deviation)
		}
	}
	return result
}

func (t *VigorTrial) CanRelease() error {
	if t.Status != Reviewed {
		return ErrInvalidTransition
	}
	report := t.GateReport()
	if !report.Passed {
		return errors.New("质量门禁未通过")
	}
	return nil
}
