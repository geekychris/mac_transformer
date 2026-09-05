package core

// Recipe is a saved Request template — everything except the input path.
// Users pick one when submitting a job (DropZone or watch folders), and the
// concrete Request is built by copying non-empty fields from the recipe
// onto the base Request.
//
// Design note: we deliberately store this as a Request (minus path) rather
// than a dedicated shape so any Request field becomes recipeable "for
// free" — no risk of the recipe schema drifting from Request.
type Recipe struct {
	Name    string  `json:"name"`
	Icon    string  `json:"icon,omitempty"` // one-glyph emoji shown next to the name
	Request Request `json:"request"`        // request template; Request.InputPath is ignored
}

// ApplyRecipe layers a recipe on top of a base Request. Recipe fields
// override base fields when set; the base is treated as user-typed values
// that shouldn't be stomped unless intentional.
//
// The specific rule is per-field:
//   - Non-empty scalars (Outputs, Layout, OutputDir, voice, etc.) always
//     win from the recipe (so the recipe defines the shape).
//   - *bool publish flags: recipe wins if set, else base.
//   - Tags: recipe replaces (not concat) — recipes are the source of truth.
//
// Callers use this to build the final Request that goes to Queue.Submit.
func ApplyRecipe(base Request, r Recipe) Request {
	out := base
	rr := r.Request

	if len(rr.Outputs) > 0 {
		out.Outputs = rr.Outputs
	}
	if rr.TTS.Engine != "" {
		out.TTS.Engine = rr.TTS.Engine
	}
	if rr.TTS.Voice != "" {
		out.TTS.Voice = rr.TTS.Voice
	}
	if rr.TTS.Speed != 0 {
		out.TTS.Speed = rr.TTS.Speed
	}
	if rr.OutputDir != "" {
		out.OutputDir = rr.OutputDir
	}
	if rr.Layout != "" {
		out.Layout = rr.Layout
	}
	if rr.PluginOverrides != nil {
		out.PluginOverrides = rr.PluginOverrides
	}
	if rr.AudioByChapter != nil {
		out.AudioByChapter = rr.AudioByChapter
	}
	if rr.PublishToBooks != nil {
		out.PublishToBooks = rr.PublishToBooks
	}
	if rr.BooksCollection != "" {
		out.BooksCollection = rr.BooksCollection
	}
	if rr.PublishToMusic != nil {
		out.PublishToMusic = rr.PublishToMusic
	}
	if rr.MusicPlaylist != "" {
		out.MusicPlaylist = rr.MusicPlaylist
	}
	if rr.MusicGenre != "" {
		out.MusicGenre = rr.MusicGenre
	}
	if rr.PublishToReadingList != nil {
		out.PublishToReadingList = rr.PublishToReadingList
	}
	if len(rr.Tags) > 0 {
		out.Tags = rr.Tags
	}
	return out
}
