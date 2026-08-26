package domain

import (
	"errors"
	"strings"
	"time"
)

func (t *VigorTrial) HasGroup(code string) bool {
	for _, group := range t.Groups {
		if group == code {
			return true
		}
	}
	return false
}

func (t *VigorTrial) ValidateIdentity() error {
	if strings.TrimSpace(t.SeedLotCode) == "" {
		return errors.New("seed_lot_code 必填")
	}
	if strings.TrimSpace(t.CropName) == "" {
		return errors.New("crop_name 必填")
	}
	if strings.TrimSpace(t.Owner) == "" {
		return errors.New("owner 必填")
	}
	seen := map[string]bool{}
	for _, group := range t.Groups {
		group = strings.TrimSpace(group)
		if group == "" {
			return errors.New("group code 不能为空")
		}
		if seen[group] {
			return errors.New("group code 不能重复")
		}
		seen[group] = true
	}
	if len(seen) == 0 {
		return errors.New("groups 必填")
	}
	return nil
}

func (t *VigorTrial) ValidateExposure(exposure Exposure) error {
	if !t.HasGroup(exposure.GroupCode) {
		return errors.New("group_code 不属于批次")
	}
	if strings.TrimSpace(exposure.Device) == "" {
		return errors.New("device 必填")
	}
	if strings.TrimSpace(exposure.Operator) == "" {
		return errors.New("operator 必填")
	}
	if strings.TrimSpace(exposure.Evidence) == "" {
		return errors.New("evidence 必填")
	}
	if exposure.StartedAt.IsZero() || exposure.EndedAt.IsZero() || !exposure.EndedAt.After(exposure.StartedAt) {
		return errors.New("老化起止时间无效")
	}
	if exposure.EndedAt.After(time.Now().UTC().Add(time.Minute)) {
		return errors.New("老化结束时间不能在未来")
	}
	if exposure.Humidity < 0 || exposure.Humidity > 100 {
		return errors.New("设备湿度超出范围")
	}
	return nil
}
