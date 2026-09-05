package cues

// EmitKind is the cue action type.
type EmitKind string

const (
	EmitSignal EmitKind = "signal"
	EmitTask   EmitKind = "task"
)

// Cue is a validated Forage Cue definition loaded from .paseka/cues/<id>.yaml.
type Cue struct {
	ID              string
	Description     string
	Emit            EmitKind
	EnergyBudget    int
	StandingTrace   string
	StandingStipend int
	SignalType      string
	SignalKind      string
	Static          map[string]string
	TitleTemplate   string
	BodyTemplate    string
	PayloadTemplate string
	// Task fields (validated on load; used when emit is task).
	Bee     string
	Intent  string
	Review  string
	Autorun bool
}

// IsStanding reports whether this cue is bound to a Standing Trail identity.
func (c Cue) IsStanding() bool {
	return c.StandingTrace != ""
}

// Summary is a list entry for paseka cue list.
type Summary struct {
	ID            string
	Description   string
	StandingTrace string
}
