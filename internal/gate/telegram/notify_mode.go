package telegram

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
	"gopkg.in/yaml.v3"
)

// NotifyMode controls whether an outbound push is sent and whether it alerts.
type NotifyMode string

const (
	NotifyOff    NotifyMode = "off"
	NotifySilent NotifyMode = "silent"
	NotifySound  NotifyMode = "sound"
)

// NotifyCategory identifies a semantic outbound push type.
type NotifyCategory int

const (
	NotifyCategoryInvites NotifyCategory = iota
	NotifyCategoryBlocked
	NotifyCategoryFailed
	NotifyCategoryReviewRequired
	NotifyCategoryReviewFinal
	NotifyCategoryCommitGate
	NotifyCategoryCompleted
)

// UnmarshalYAML accepts bool (true→sound, false→off) or string (off/silent/sound).
func (m *NotifyMode) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return fmt.Errorf("notify mode: nil node")
	}
	switch value.Kind {
	case yaml.ScalarNode:
		v := strings.ToLower(strings.TrimSpace(value.Value))
		switch v {
		case "false", "off":
			*m = NotifyOff
		case "true", "sound":
			*m = NotifySound
		case "silent":
			*m = NotifySilent
		default:
			return fmt.Errorf("notify mode: invalid value %q (use off, silent, sound, or bool)", value.Value)
		}
		return nil
	default:
		return fmt.Errorf("notify mode: expected scalar, got %v", value.Kind)
	}
}

// Enabled reports whether pushes for this mode should be delivered.
func (m NotifyMode) Enabled() bool {
	return m != NotifyOff
}

// Silent reports whether Telegram should suppress notification sound/vibration.
func (m NotifyMode) Silent() bool {
	return m == NotifySilent
}

// Mode returns the effective notify mode for a category (defaults when unset).
func (n NotifyConfig) Mode(cat NotifyCategory) NotifyMode {
	switch cat {
	case NotifyCategoryInvites:
		return n.modeOrDefault(n.Invites, NotifySound)
	case NotifyCategoryBlocked:
		return n.modeOrDefault(n.Blocked, NotifySound)
	case NotifyCategoryFailed:
		return n.modeOrDefault(n.Failed, NotifySound)
	case NotifyCategoryReviewRequired:
		return n.modeOrDefault(n.ReviewRequired, NotifySound)
	case NotifyCategoryReviewFinal:
		return n.modeOrDefault(n.ReviewFinal, NotifySound)
	case NotifyCategoryCommitGate:
		return n.modeOrDefault(n.CommitGate, NotifyOff)
	case NotifyCategoryCompleted:
		return n.modeOrDefault(n.Completed, NotifySilent)
	default:
		return NotifyOff
	}
}

func (n NotifyConfig) modeOrDefault(field *NotifyMode, def NotifyMode) NotifyMode {
	if field == nil {
		return def
	}
	return *field
}

// applyLegacyWaitingReview maps deprecated waiting_review to review_required/review_final.
func (n *NotifyConfig) applyLegacyWaitingReview() {
	if n.WaitingReview == nil {
		return
	}
	legacy := *n.WaitingReview
	if n.ReviewRequired == nil {
		n.ReviewRequired = &legacy
	}
	if n.ReviewFinal == nil {
		n.ReviewFinal = &legacy
	}
}

// classifyWaitingReview splits waiting_review by review policy.
func classifyWaitingReview(task taskledger.TaskSnapshot) NotifyCategory {
	if taskledger.IsFinalReviewTask(task) {
		return NotifyCategoryReviewFinal
	}
	if taskledger.IsReviewGate(task) {
		return NotifyCategoryReviewRequired
	}
	return NotifyCategoryCommitGate
}

// classifyTaskStatus maps a task status (+ snapshot) to a notify category.
func classifyTaskStatus(task taskledger.TaskSnapshot, status protocol.TaskStatus) (NotifyCategory, bool) {
	switch status {
	case protocol.TaskStatusBlocked:
		return NotifyCategoryBlocked, true
	case protocol.TaskStatusFailed:
		return NotifyCategoryFailed, true
	case protocol.TaskStatusWaitingReview:
		return classifyWaitingReview(task), true
	default:
		return 0, false
	}
}
