package httpapi

import (
	"errors"
	"strings"
)

func validateCommand(input command, needsRevision bool) error {
	if strings.TrimSpace(input.RequestID) == "" {
		return errors.New("request_id 必填")
	}
	if len(input.RequestID) > 128 {
		return errors.New("request_id 过长")
	}
	if strings.TrimSpace(input.Actor) == "" {
		return errors.New("actor 必填")
	}
	if strings.TrimSpace(input.Role) == "" {
		return errors.New("role 必填")
	}
	if needsRevision && input.ExpectedRevision < 1 {
		return errors.New("expected_revision 必须为正整数")
	}
	return nil
}

func validateEmbedded(input command, needsRevision bool) error {
	return validateCommand(input, needsRevision)
}
