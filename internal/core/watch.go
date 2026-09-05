package core

// WatchFolder describes a directory the app monitors for new files. When a
// file with a matching extension appears (or is modified), a job is
// submitted using the named recipe.
type WatchFolder struct {
	Enabled    bool     `json:"enabled"`
	Path       string   `json:"path"`
	Recipe     string   `json:"recipe,omitempty"`
	Extensions []string `json:"extensions,omitempty"` // empty = accept any known input type
	// Recursive tracks subdirectories too. Off by default because most
	// "inbox" folders are flat and recursive watches can be surprising.
	Recursive bool `json:"recursive,omitempty"`
}
