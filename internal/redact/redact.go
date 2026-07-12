// Package redact implements payload redaction: scrubbing
// activity/workflow/signal payload bytes before a history is ever persisted
// to a corpus, since histories sampled from a live cluster can contain PII.
package redact

import (
	commonpb "go.temporal.io/api/common/v1"
	historypb "go.temporal.io/api/history/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Scrubber transforms a single payload before it's persisted. Implement this
// to plug in custom redaction; see profiles.go for the built-in none/default/hash
// profiles used by `replaygate sample`.
type Scrubber interface {
	Scrub(p *commonpb.Payload) *commonpb.Payload
}

var payloadFullName = (&commonpb.Payload{}).ProtoReflect().Descriptor().FullName()

// RedactHistory walks every event in hist and rewrites every commonpb.Payload
// found anywhere in the tree — activity/workflow/signal inputs and results,
// headers, memos, failure details, whatever future event types add — via s.
// It mutates hist in place.
//
// This is deliberately generic (protobuf reflection over the whole event
// tree) rather than an explicit switch over every HistoryEvent attribute
// type: the SDK adds new event/attribute types over time, and a hardcoded
// list would silently stop redacting whatever it forgot.
func RedactHistory(hist *historypb.History, s Scrubber) {
	if hist == nil || s == nil {
		return
	}
	for _, e := range hist.Events {
		if e != nil {
			walk(e.ProtoReflect(), s)
		}
	}
}

func walk(m protoreflect.Message, s Scrubber) {
	if !m.IsValid() {
		return
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Kind() != protoreflect.MessageKind && fd.Kind() != protoreflect.GroupKind {
			return true
		}
		switch {
		case fd.IsMap():
			if fd.MapValue().Kind() != protoreflect.MessageKind && fd.MapValue().Kind() != protoreflect.GroupKind {
				return true
			}
			v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
				visit(mv.Message(), s)
				return true
			})
		case fd.IsList():
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				visit(list.Get(i).Message(), s)
			}
		default:
			visit(v.Message(), s)
		}
		return true
	})
}

func visit(m protoreflect.Message, s Scrubber) {
	if m.Descriptor().FullName() == payloadFullName {
		p, ok := m.Interface().(*commonpb.Payload)
		if !ok || p == nil {
			return
		}
		if scrubbed := s.Scrub(p); scrubbed != nil && scrubbed != p {
			// Field-by-field, not *p = *scrubbed: Payload embeds a protobuf
			// MessageState (with a mutex) that must never be copied by value.
			p.Metadata = scrubbed.Metadata
			p.Data = scrubbed.Data
		}
		return
	}
	walk(m, s)
}
