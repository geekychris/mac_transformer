package core

// CapabilityView is the JSON-friendly projection of plugin.Capability used
// on the wire. Keeping this in core avoids httpapi depending on plugin
// internals for its response types.
type CapabilityView struct {
	From     []string `json:"from"`
	To       []string `json:"to"`
	Priority int      `json:"priority"`
}
