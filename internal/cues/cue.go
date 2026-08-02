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
	SignalType      string
	SignalKind      string
	Static          map[string]string
	TitleTemplate   string
	BodyTemplate    string
	PayloadTemplate string
	// Task fields (validated on load; run supports signal only in this slice).
	Bee     string
	Intent  string
	Review  string
	Autorun bool
}

// Summary is a list entry for paseka cue list.
type Summary struct {
	ID          string
	Description string
}
