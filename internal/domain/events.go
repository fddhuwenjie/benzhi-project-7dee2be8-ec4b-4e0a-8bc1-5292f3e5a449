package domain

import "time"

func NewAudit(t *VigorTrial, typ, actor, request string, data map[string]any) AuditEvent {
	e := AuditEvent{At: time.Now().UTC(), Type: typ, Actor: actor, RequestID: request, Revision: t.Revision, Data: data}
	prev := ""
	if n := len(t.Audit); n > 0 {
		prev = t.Audit[n-1].Digest
	}
	e.Digest = auditEventDigest(prev, e)
	return e
}

func auditEventDigest(previous string, event AuditEvent) string {
	event.Digest = ""
	if event.Type == "TRIAL_ARCHIVED" && event.Data != nil {
		data := make(map[string]any, len(event.Data))
		for key, value := range event.Data {
			if key != "archive_digest" {
				data[key] = value
			}
		}
		if len(data) == 0 {
			event.Data = nil
		} else {
			event.Data = data
		}
	}
	return Digest(struct {
		Prev  string
		Event AuditEvent
	}{previous, event})
}
