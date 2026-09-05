package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geekychris/mac_transformer/internal/plugin"
)

// Dispatcher turns a Request into one artifact per requested OutputKind by
// walking the plugin registry.
//
// The rules:
//   - For each requested output kind, compute a desired concrete format
//     (pdf/txt/wav/mp3/aiff).
//   - Ask the registry for the highest-priority available plugin that
//     converts input_format -> desired_format directly. If none, and the
//     desired output is audio, try a two-step chain: input -> txt -> audio.
//   - Honor per-conversion plugin overrides.
type Dispatcher struct {
	Registry *plugin.Registry
	Config   *Store

	// Progress, if set, is invoked after every observable state transition
	// on the Job (status flip, step start/end, artifact append). The queue
	// wires this to push SSE events so the frontend re-renders in real time.
	Progress func(*Job)
}

func (d *Dispatcher) tick(job *Job) {
	if d.Progress != nil {
		d.Progress(job)
	}
}

// Run executes a Job in-place, populating Artifacts and Steps. It's a
// blocking call; the queue is responsible for running it in a goroutine.
func (d *Dispatcher) Run(ctx context.Context, job *Job) {
	job.Status = StatusRunning
	job.Started = timeNow()
	defer func() {
		if job.Status == StatusRunning {
			// Nothing else set a terminal status — treat as complete.
			job.Status = StatusCompleted
		}
		job.Ended = timeNow()
		d.tick(job)
	}()

	cfg := d.Config.Get()
	req := &job.Request

	// Resolve TTS defaults.
	if req.TTS.Engine == "" {
		req.TTS.Engine = cfg.DefaultTTSEngine
	}
	if req.TTS.Voice == "" {
		if req.TTS.Engine == "say" {
			req.TTS.Voice = cfg.DefaultSayVoice
		} else {
			req.TTS.Voice = cfg.DefaultKokoroVoice
		}
	}
	if req.TTS.Speed == 0 {
		req.TTS.Speed = cfg.DefaultSpeed
	}

	// Resolve layout and output dir. "beside" ignores OutputDir entirely
	// and writes right next to the input document.
	layout := req.Layout
	if layout == "" {
		layout = cfg.DefaultLayout
	}
	if layout == "" {
		layout = LayoutFlat
	}
	baseDir := req.OutputDir
	if baseDir == "" {
		baseDir = cfg.OutputDir
	}

	inFmt := DetectFormat(req.InputPath)
	if inFmt == "" {
		d.fail(job, fmt.Sprintf("unknown input format for %s", req.InputPath))
		return
	}

	// Merge overrides: request-level takes precedence over config-level.
	overrides := map[string]string{}
	for k, v := range cfg.PluginOverrides {
		overrides[k] = v
	}
	for k, v := range req.PluginOverrides {
		overrides[k] = v
	}

	inputStem := strings.TrimSuffix(filepath.Base(req.InputPath), filepath.Ext(req.InputPath))

	// Resolve publish options and locate the bundled scripts dir (any
	// bundled plugin's Dir works — they all live in the same directory).
	pubOpts := resolvePublish(cfg, req)
	scriptDir := ""
	for _, lp := range d.Registry.All() {
		if lp.Manifest.Bundled {
			scriptDir = lp.Manifest.Dir
			break
		}
	}

	// Pre-populate the step list so the frontend can render a checklist
	// with every output kind visible immediately, not one entry per
	// finished step.
	job.Steps = make([]StepReport, len(req.Outputs))
	for i, kind := range req.Outputs {
		job.Steps[i] = StepReport{Kind: kind, Status: StepPending}
	}
	d.tick(job)

	for i, kind := range req.Outputs {
		// Honor cancellation between steps. In-flight plugin execution
		// (subprocess exec) also honors ctx via CommandContext.
		if ctx.Err() != nil {
			job.Status = StatusCancelled
			if job.Message == "" {
				job.Message = "cancelled"
			}
			return
		}
		step := &job.Steps[i]

		desired := DesiredFormat(kind, req.TTS.Engine)
		if desired == "" {
			step.Status = StepFailed
			step.Error = fmt.Sprintf("unknown output kind %q", kind)
			step.Started, step.Ended = timeNow(), timeNow()
			d.tick(job)
			continue
		}

		step.Status = StepRunning
		step.Started = timeNow()
		step.Message = plannedMessage(kind, desired, inFmt)
		d.tick(job)

		outPath, err := resolveArtifactPath(req.InputPath, inputStem, string(desired), baseDir, layout)
		if err != nil {
			step.Status = StepFailed
			step.Error = err.Error()
			step.Ended = timeNow()
			job.Status = StatusFailed
			job.Message = err.Error()
			d.tick(job)
			continue
		}
		// Summary path: independent output kind; needs text extraction
		// (or reads text directly), then hits an LLM endpoint.
		if kind == OutputSummary {
			outPath, err := resolveArtifactPath(req.InputPath, inputStem, "summary.md", baseDir, layout)
			if err == nil {
				art, chain, serr := d.produceSummary(ctx, req.InputPath, inFmt, outPath, overrides, cfg)
				step.Ended = timeNow()
				step.Chain = chain
				if len(chain) > 0 {
					step.Plugin = chain[len(chain)-1]
				}
				if serr != nil {
					step.Status = StepFailed
					step.Error = serr.Error()
					job.Status = StatusFailed
					job.Message = serr.Error()
				} else {
					step.Status = StepCompleted
					art.Kind = kind
					applyPostProcessing(ctx, &art, pubOpts, scriptDir)
					job.Artifacts = append(job.Artifacts, art)
				}
			} else {
				step.Status = StepFailed
				step.Error = err.Error()
				step.Ended = timeNow()
			}
			d.tick(job)
			continue
		}

		// Chapter-split audio path: only when the caller opts in AND the
		// requested output is audio. Everything else takes the single-file
		// path.
		if kind == OutputAudio && chapterMode(cfg, req) {
			arts, chain, err := d.produceChapters(ctx, req.InputPath, inFmt, string(desired),
				baseDir, layout, inputStem, overrides, req)
			step.Ended = timeNow()
			step.Chain = chain
			if len(chain) > 0 {
				step.Plugin = chain[len(chain)-1]
			}
			if err != nil {
				step.Status = StepFailed
				step.Error = err.Error()
				step.Message = ""
				job.Status = StatusFailed
				job.Message = err.Error()
				d.tick(job)
				continue
			}
			step.Status = StepCompleted
			step.Message = ""
			for _, a := range arts {
				a.Kind = kind
				applyPostProcessing(ctx, &a, pubOpts, scriptDir)
				job.Artifacts = append(job.Artifacts, a)
			}
			d.tick(job)
			continue
		}

		art, chain, err := d.produce(ctx, req.InputPath, inFmt, string(desired), outPath, overrides, req)
		step.Ended = timeNow()
		step.Chain = chain
		if len(chain) > 0 {
			step.Plugin = chain[len(chain)-1]
		}
		if err != nil {
			step.Status = StepFailed
			step.Error = err.Error()
			step.Message = ""
			job.Status = StatusFailed
			job.Message = err.Error()
			d.tick(job)
			continue
		}
		step.Status = StepCompleted
		step.Message = ""
		art.Kind = kind
		// PDF metadata: stamp Title/Author so Books.app and Reader show
		// real names instead of the filename. Silent on failure — the
		// PDF is fine either way.
		if art.Format == FormatPDF {
			title := req.PDFTitle
			if title == "" {
				title = inputStem
			}
			author := req.PDFAuthor
			if author == "" {
				author = cfg.DefaultPDFAuthor
			}
			_ = stampPDFMetadata(art.Path, title, author, req.PDFSubject, req.Tags)
		}
		// ID3 for single-file audio too (album/artist/track=1/1).
		if isAudioFormat(art.Format) {
			_ = tagAudioFile(ctx, art.Path, inputStem, inputStem, "Mac Transformer", req.MusicGenre, 1, 1)
			if art.Format == FormatMP3 {
				if cover, err := generateCoverImage(inputStem); err == nil {
					_ = embedMP3Cover(art.Path, cover)
				}
			}
		}
		// Post-processing: tags, Books, Music. Best-effort — populates
		// art.Publish; doesn't fail the step.
		applyPostProcessing(ctx, &art, pubOpts, scriptDir)
		job.Artifacts = append(job.Artifacts, art)
		d.tick(job)
	}
}

// resolveArtifactPath computes where a single artifact should land, given
// the input path, the output stem, its target extension, the user's chosen
// baseDir, and the chosen layout. Creates any missing directories.
func resolveArtifactPath(inputPath, stem, ext, baseDir string, layout OutputLayout) (string, error) {
	var dir string
	switch layout {
	case LayoutBeside:
		dir = filepath.Dir(inputPath)
	case LayoutSubfolder:
		if baseDir == "" {
			return "", fmt.Errorf("layout=subfolder requires an output_dir")
		}
		dir = filepath.Join(baseDir, stem)
	case LayoutFlat, "":
		if baseDir == "" {
			return "", fmt.Errorf("layout=flat requires an output_dir")
		}
		dir = baseDir
	default:
		return "", fmt.Errorf("unknown layout %q", layout)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("prepare %s: %w", dir, err)
	}
	return filepath.Join(dir, stem+"."+ext), nil
}

// produceSummary extracts text (or reads it directly for text-native
// formats), sends it to the configured LLM endpoint, and writes the
// summary markdown to outPath. Returns the artifact plus the plugin chain
// used. Truncates text at ~50k chars to stay within typical model contexts;
// for longer docs users should split first.
func (d *Dispatcher) produceSummary(
	ctx context.Context,
	inputPath string, inFmt Format, outPath string,
	overrides map[string]string, cfg Config,
) (Artifact, []string, error) {
	if cfg.SummarizerURL == "" {
		return Artifact{}, nil, fmt.Errorf("summarizer_url not set in Settings")
	}
	var text string
	var chain []string
	switch inFmt {
	case FormatMD, "markdown", FormatADOC, "asciidoc", "rst", "org", "txt", "html":
		raw, err := os.ReadFile(inputPath)
		if err != nil {
			return Artifact{}, chain, err
		}
		text = string(raw)
	default:
		// Extract via the text plugin.
		txt, err := d.pickTextPlugin(inFmt, overrides)
		if err != nil {
			return Artifact{}, chain, err
		}
		chain = append(chain, txt.Manifest.Name)
		tmpDir, err := os.MkdirTemp("", "mac-transformer-summary-")
		if err != nil {
			return Artifact{}, chain, err
		}
		defer os.RemoveAll(tmpDir)
		txtPath := filepath.Join(tmpDir, "in.txt")
		if _, err := d.runOne(ctx, txt, inputPath, txtPath, &Request{}); err != nil {
			return Artifact{}, chain, err
		}
		raw, err := os.ReadFile(txtPath)
		if err != nil {
			return Artifact{}, chain, err
		}
		text = string(raw)
	}
	if len(text) > 50000 {
		text = text[:50000] + "\n\n[... truncated for context window ...]"
	}
	summary, err := summarize(ctx, cfg, text)
	if err != nil {
		return Artifact{}, chain, err
	}
	if err := os.WriteFile(outPath, []byte(summary), 0o644); err != nil {
		return Artifact{}, chain, err
	}
	info, err := os.Stat(outPath)
	if err != nil {
		return Artifact{}, chain, err
	}
	chain = append(chain, "summarizer")
	return Artifact{
		Format: "md",
		Path:   outPath,
		Plugin: "summarizer",
		Bytes:  info.Size(),
	}, chain, nil
}

// chapterMode returns whether the caller has asked for chapter-split audio.
// Request override wins over config default.
func chapterMode(cfg Config, req *Request) bool {
	if req.AudioByChapter != nil {
		return *req.AudioByChapter
	}
	return cfg.AudioByChapter
}

// produceChapters extracts text from the input, splits it into chapters,
// synthesizes each chapter separately, tags the resulting audio files
// with ID3 metadata, and returns the list of produced artifacts.
//
// Naming: <stem>-01-<slug>.<ext>, <stem>-02-<slug>.<ext>, …. Chapter titles
// are shown in Music as track titles; the source doc's stem becomes the album.
func (d *Dispatcher) produceChapters(
	ctx context.Context,
	inputPath string, inFmt Format, desired string,
	baseDir string, layout OutputLayout, inputStem string,
	overrides map[string]string, req *Request,
) ([]Artifact, []string, error) {
	// Step 1: get the text we'll split.
	// For markup formats we already own (md/adoc/rst/org/txt), read the
	// source directly so heading markers survive. For binary formats
	// (docx/pptx/pdf/…), extract via pandoc first — chapter detection is
	// then heuristic-only, but that's better than nothing.
	tmpDir, err := os.MkdirTemp("", "mac-transformer-chapters-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(tmpDir)

	var rawText []byte
	var chain []string
	switch inFmt {
	case FormatMD, FormatADOC, "markdown", "asciidoc", "rst", "org", "txt", "html":
		rawText, err = os.ReadFile(inputPath)
		if err != nil {
			return nil, nil, err
		}
	default:
		txt, err := d.pickTextPlugin(inFmt, overrides)
		if err != nil {
			return nil, nil, err
		}
		chain = append(chain, txt.Manifest.Name)
		txtPath := filepath.Join(tmpDir, inputStem+".txt")
		if _, err := d.runOne(ctx, txt, inputPath, txtPath, req); err != nil {
			return nil, chain, err
		}
		rawText, err = os.ReadFile(txtPath)
		if err != nil {
			return nil, chain, err
		}
	}
	chapters := DetectChapters(string(rawText), string(inFmt))
	if len(chapters) == 0 {
		return nil, chain, fmt.Errorf("no content extracted")
	}

	// Step 2: for each chapter, write a text file, synthesise audio,
	// tag it. Fan-out is serial to be gentle on the TTS server.
	tts, err := d.pickAudioPlugin(desired, overrides)
	if err != nil {
		return nil, chain, err
	}
	chain = append(chain, tts.Manifest.Name)
	total := len(chapters)
	var artifacts []Artifact
	for i, ch := range chapters {
		if ctx.Err() != nil {
			return artifacts, chain, ctx.Err()
		}
		chTxtPath := filepath.Join(tmpDir, fmt.Sprintf("ch%02d.txt", i+1))
		if err := os.WriteFile(chTxtPath, []byte(ch.Body), 0o644); err != nil {
			return artifacts, chain, err
		}

		outStem := fmt.Sprintf("%s-%02d-%s", inputStem, ch.Number, slugify(ch.Title))
		outPath, err := resolveArtifactPath(inputPath, outStem, desired, baseDir, layout)
		if err != nil {
			return artifacts, chain, err
		}
		art, err := d.runOne(ctx, tts, chTxtPath, outPath, req)
		if err != nil {
			return artifacts, chain, fmt.Errorf("chapter %d (%s): %w", ch.Number, ch.Title, err)
		}
		_ = tagAudioFile(ctx, art.Path, ch.Title, inputStem, "Mac Transformer", req.MusicGenre, ch.Number, total)
		if strings.EqualFold(desired, "mp3") {
			if cover, err := generateCoverImage(inputStem); err == nil {
				_ = embedMP3Cover(art.Path, cover)
			}
		}
		artifacts = append(artifacts, art)
	}
	return artifacts, chain, nil
}

// pickTextPlugin returns the highest-priority available plugin that can
// extract text from the input format. Wraps pickPlugins.
func (d *Dispatcher) pickTextPlugin(inFmt Format, overrides map[string]string) (*plugin.LoadedPlugin, error) {
	cands := d.pickPlugins(string(inFmt), string(FormatTXT), overrides)
	if len(cands) == 0 {
		return nil, fmt.Errorf("no plugin can extract text from %s", inFmt)
	}
	return cands[0], nil
}

func (d *Dispatcher) pickAudioPlugin(desired string, overrides map[string]string) (*plugin.LoadedPlugin, error) {
	cands := d.pickPlugins(string(FormatTXT), desired, overrides)
	if len(cands) == 0 {
		return nil, fmt.Errorf("no TTS plugin for %s", desired)
	}
	return cands[0], nil
}

// slugify turns a chapter title into a filename-safe stem.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
		if b.Len() >= 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "untitled"
	}
	return out
}

// plannedMessage is a short human-readable summary of what the step is
// about to do, shown while it's running. Kept intentionally generic — the
// real plugin name lands in step.Plugin once produce() returns.
func plannedMessage(kind OutputKind, desired Format, inFmt Format) string {
	switch kind {
	case OutputPDF:
		return fmt.Sprintf("converting %s → pdf", inFmt)
	case OutputText:
		return fmt.Sprintf("extracting text from %s", inFmt)
	case OutputAudio:
		if inFmt == FormatTXT {
			return fmt.Sprintf("synthesizing %s", desired)
		}
		return fmt.Sprintf("extracting text, then synthesizing %s", desired)
	}
	return ""
}

func (d *Dispatcher) fail(job *Job, msg string) {
	job.Status = StatusFailed
	job.Message = msg
}

// produce is the workhorse. It resolves a plugin chain input -> desiredFormat
// and executes it, returning the produced Artifact and the ordered list of
// plugin names used. On plugin failure it falls back to the next-highest
// priority plugin capable of the same conversion.
func (d *Dispatcher) produce(
	ctx context.Context,
	inputPath string, inFmt Format, desired string, outPath string,
	overrides map[string]string, req *Request,
) (Artifact, []string, error) {
	// Try direct: walk every capable plugin in priority order.
	candidates := d.pickPlugins(string(inFmt), desired, overrides)
	var lastErr error
	var tried []string
	for _, lp := range candidates {
		art, err := d.runOne(ctx, lp, inputPath, outPath, req)
		tried = append(tried, lp.Manifest.Name)
		if err == nil {
			return art, tried, nil
		}
		lastErr = fmt.Errorf("%s: %w", lp.Manifest.Name, err)
	}

	// If no direct plugin (or all failed) and this is an audio output, try
	// the chained route: input -> txt -> desired.
	if desired == string(FormatWAV) || desired == string(FormatMP3) || desired == string(FormatAIFF) {
		art, chain, err := d.chainViaText(ctx, inputPath, inFmt, desired, outPath, overrides, req)
		if err == nil {
			return art, chain, nil
		}
		if lastErr == nil {
			return Artifact{}, chain, err
		}
		return Artifact{}, append(tried, chain...), fmt.Errorf("all direct plugins failed (%v); chain also failed: %w", lastErr, err)
	}

	if lastErr != nil {
		return Artifact{}, tried, fmt.Errorf("all plugins failed for %s -> %s: %w", inFmt, desired, lastErr)
	}
	return Artifact{}, nil, fmt.Errorf("no plugin available: %s -> %s", inFmt, desired)
}

func (d *Dispatcher) chainViaText(
	ctx context.Context,
	inputPath string, inFmt Format, desired string, outPath string,
	overrides map[string]string, req *Request,
) (Artifact, []string, error) {
	// 1) input -> txt: walk candidates in priority order, take the first
	//    plugin that actually produces a file. This is where Word-fails ->
	//    pandoc-text-wins recovery happens.
	txtCands := d.pickPlugins(string(inFmt), string(FormatTXT), overrides)
	if len(txtCands) == 0 {
		return Artifact{}, nil, fmt.Errorf("no plugin can extract text from %s", inFmt)
	}
	ttsCands := d.pickPlugins(string(FormatTXT), desired, overrides)
	if len(ttsCands) == 0 {
		return Artifact{}, nil, fmt.Errorf("no plugin can synthesize %s from txt", desired)
	}

	inputStem := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	tmpDir, err := os.MkdirTemp("", "mac-transformer-")
	if err != nil {
		return Artifact{}, nil, err
	}
	defer os.RemoveAll(tmpDir)
	txtPath := filepath.Join(tmpDir, inputStem+".txt")

	var txtPluginUsed *plugin.LoadedPlugin
	var lastTxtErr error
	for _, lp := range txtCands {
		if _, err := d.runOne(ctx, lp, inputPath, txtPath, req); err == nil {
			txtPluginUsed = lp
			break
		} else {
			lastTxtErr = fmt.Errorf("%s: %w", lp.Manifest.Name, err)
		}
	}
	if txtPluginUsed == nil {
		return Artifact{}, nil, fmt.Errorf("text extraction failed: %w", lastTxtErr)
	}

	var lastTTSErr error
	for _, lp := range ttsCands {
		art, err := d.runOne(ctx, lp, txtPath, outPath, req)
		if err == nil {
			art.Kind = OutputAudio
			return art, []string{txtPluginUsed.Manifest.Name, lp.Manifest.Name}, nil
		}
		lastTTSErr = fmt.Errorf("%s: %w", lp.Manifest.Name, err)
	}
	return Artifact{}, []string{txtPluginUsed.Manifest.Name}, fmt.Errorf("tts failed: %w", lastTTSErr)
}

func (d *Dispatcher) runOne(
	ctx context.Context,
	lp *plugin.LoadedPlugin,
	inputPath string, outPath string,
	req *Request,
) (Artifact, error) {
	inputStem := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputStem := strings.TrimSuffix(filepath.Base(outPath), filepath.Ext(outPath))
	inExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(inputPath), "."))
	outExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(outPath), "."))
	outDir := filepath.Dir(outPath)

	tctx := &plugin.TemplateContext{
		Input:      inputPath,
		InputStem:  inputStem,
		InputExt:   inExt,
		Output:     outPath,
		OutputStem: outputStem,
		OutputExt:  outExt,
		OutputDir:  outDir,
		PluginDir:  lp.Manifest.Dir,
		Voice:      req.TTS.Voice,
		Speed:      req.TTS.Speed,
	}

	// If the plugin needs input_text (TTS from a txt file), slurp it.
	if inExt == "txt" && strings.Contains(concat(lp.Manifest.Run.BodyTemplate, marshalJSON(lp.Manifest.Run.BodyJSON), strings.Join(lp.Manifest.Run.Argv, " ")), "input_text") {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return Artifact{}, fmt.Errorf("read %s: %w", inputPath, err)
		}
		tctx.InputText = string(data)
	}

	produced, err := plugin.Execute(ctx, lp.Manifest, tctx)
	if err != nil {
		return Artifact{}, err
	}
	info, err := os.Stat(produced)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		Format: Format(outExt),
		Path:   produced,
		Plugin: lp.Manifest.Name,
		Bytes:  info.Size(),
	}, nil
}

// pickPlugins returns all available plugins that can convert from -> to,
// ordered highest priority first. An override for from->to short-circuits
// to a single-element slice (with no fallback) — the user has explicitly
// chosen that plugin.
func (d *Dispatcher) pickPlugins(from, to string, overrides map[string]string) []*plugin.LoadedPlugin {
	key := from + "->" + to
	if name, ok := overrides[key]; ok {
		lp := d.Registry.ByName(name)
		if lp == nil || !lp.Available {
			return nil
		}
		return []*plugin.LoadedPlugin{lp}
	}
	return d.Registry.Match(from, to)
}

func concat(parts ...string) string { return strings.Join(parts, "\n") }

func marshalJSON(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	// Cheap contains-check; don't need real JSON marshalling.
	var b strings.Builder
	walk(&b, m)
	return b.String()
}

func walk(b *strings.Builder, v any) {
	switch x := v.(type) {
	case string:
		b.WriteString(x)
	case map[string]any:
		for _, vv := range x {
			walk(b, vv)
			b.WriteByte(' ')
		}
	case []any:
		for _, vv := range x {
			walk(b, vv)
			b.WriteByte(' ')
		}
	default:
		fmt.Fprintf(b, "%v", v)
	}
}
