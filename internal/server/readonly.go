package server

type ReadOnlyMode struct {
	Enabled bool
}

func NewReadOnlyMode(enabled bool) *ReadOnlyMode {
	return &ReadOnlyMode{Enabled: enabled}
}

var writeToolNames = []string{
	"acknowledge_alert_group",
	"resolve_alert_group",
	"silence_alert_group",
	"unresolve_alert_group",
}

func (r *ReadOnlyMode) IsWriteTool(name string) bool {
	for _, wt := range writeToolNames {
		if wt == name {
			return true
		}
	}
	return false
}

func (r *ReadOnlyMode) ShouldRegisterTool(name string) bool {
	if !r.Enabled {
		return true
	}
	return !r.IsWriteTool(name)
}
