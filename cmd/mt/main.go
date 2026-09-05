// mt is a thin CLI over the Mac Transformer REST API. It's not a
// replacement for the GUI — it's a scripting convenience so shell
// pipelines / Alfred / Raycast / launchd can hand files to the running
// server. Requires the app (or headless server) to be running.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultBase = "http://127.0.0.1:8771"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "transform", "t":
		cmdTransform(os.Args[2:])
	case "jobs", "list", "ls":
		cmdJobs(os.Args[2:])
	case "plugins":
		cmdPlugins(os.Args[2:])
	case "watch":
		cmdEvents(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `mt — command-line client for Mac Transformer

USAGE
  mt transform FILE [FILE...] [flags]   submit one or more files
  mt jobs                               list all jobs
  mt plugins                            list loaded plugins
  mt watch                              stream job events (until Ctrl-C)

TRANSFORM FLAGS
  -o, --outputs LIST     comma-separated: pdf,text,audio (default: pdf)
  -r, --recipe NAME      apply a saved recipe (overrides field flags)
      --tts ENGINE       kokoro | say
      --voice NAME       Kokoro or Say voice
      --speed N          0.5..2.0
      --layout LAYOUT    beside | flat | subfolder
      --output-dir DIR   where to write (beside layout ignores this)
      --tags LIST        comma-separated Finder tags
      --books            publish PDF to Books
      --music            publish audio to Music
      --reading-list     add PDF to Safari Reading List
      --chapters         split audio by chapter (needs ffmpeg)
      --force            skip duplicate detection
      --wait             block until finished, print artifact paths
      --host URL         override server URL (default: %s)
`, defaultBase)
}

func cmdTransform(args []string) {
	fs := flag.NewFlagSet("transform", flag.ExitOnError)
	var (
		outputs     = fs.String("outputs", "pdf", "")
		recipe      = fs.String("recipe", "", "")
		ttsEngine   = fs.String("tts", "", "")
		voice       = fs.String("voice", "", "")
		speed       = fs.Float64("speed", 0, "")
		layout      = fs.String("layout", "", "")
		outputDir   = fs.String("output-dir", "", "")
		tagsStr     = fs.String("tags", "", "")
		books       = fs.Bool("books", false, "")
		music       = fs.Bool("music", false, "")
		readingList = fs.Bool("reading-list", false, "")
		chapters    = fs.Bool("chapters", false, "")
		force       = fs.Bool("force", false, "")
		wait        = fs.Bool("wait", false, "")
		host        = fs.String("host", defaultBase, "")
	)
	fs.StringVar(outputs, "o", "pdf", "")
	fs.StringVar(recipe, "r", "", "")
	// Go's flag pkg stops at the first positional. Rearrange so all
	// flags come first — makes `mt transform file.pdf --wait` DTRT.
	_ = fs.Parse(reorderFlags(args))

	files := fs.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no files given")
		os.Exit(2)
	}

	var tags []string
	if *tagsStr != "" {
		for _, t := range strings.Split(*tagsStr, ",") {
			if s := strings.TrimSpace(t); s != "" {
				tags = append(tags, s)
			}
		}
	}

	for _, path := range files {
		abs, err := filepath.Abs(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			continue
		}
		body := map[string]any{
			"path":                    abs,
			"outputs":                 strings.Split(*outputs, ","),
			"force":                   *force,
			"publish_to_books":        *books,
			"publish_to_music":        *music,
			"publish_to_reading_list": *readingList,
			"audio_by_chapter":        *chapters,
		}
		if *recipe != "" {
			body["recipe"] = *recipe
		}
		if *ttsEngine != "" || *voice != "" || *speed != 0 {
			tts := map[string]any{}
			if *ttsEngine != "" {
				tts["engine"] = *ttsEngine
			}
			if *voice != "" {
				tts["voice"] = *voice
			}
			if *speed != 0 {
				tts["speed"] = *speed
			}
			body["tts"] = tts
		}
		if *layout != "" {
			body["layout"] = *layout
		}
		if *outputDir != "" {
			body["output_dir"] = *outputDir
		}
		if len(tags) > 0 {
			body["tags"] = tags
		}
		buf, _ := json.Marshal(body)
		resp, err := http.Post(*host+"/transform", "application/json", bytes.NewReader(buf))
		if err != nil {
			fatalf("submit %s: %v", abs, err)
		}
		if resp.StatusCode >= 400 {
			out, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fatalf("submit %s: %s: %s", abs, resp.Status, string(out))
		}
		var job map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&job)
		resp.Body.Close()
		if resp.Header.Get("X-Duplicate-Of") != "" {
			fmt.Printf("%s → already processed (job %s). Pass --force to re-run.\n", abs, job["id"])
			continue
		}
		fmt.Printf("%s → %s\n", abs, job["id"])

		if *wait {
			final := pollUntilDone(*host, job["id"].(string))
			for _, a := range asSlice(final["artifacts"]) {
				m := a.(map[string]any)
				fmt.Printf("  %s\n", m["path"])
			}
			if s, _ := final["status"].(string); s == "failed" {
				fmt.Fprintf(os.Stderr, "  failed: %s\n", final["message"])
				os.Exit(1)
			}
		}
	}
}

func cmdJobs(args []string) {
	fs := flag.NewFlagSet("jobs", flag.ExitOnError)
	host := fs.String("host", defaultBase, "")
	_ = fs.Parse(args)
	resp, err := http.Get(*host + "/jobs")
	if err != nil {
		fatalf("%v", err)
	}
	defer resp.Body.Close()
	var jobs []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&jobs)
	for _, j := range jobs {
		fmt.Printf("%s  %-10s  %s\n", j["id"], j["status"], j["request"].(map[string]any)["path"])
	}
}

func cmdPlugins(args []string) {
	fs := flag.NewFlagSet("plugins", flag.ExitOnError)
	host := fs.String("host", defaultBase, "")
	_ = fs.Parse(args)
	resp, err := http.Get(*host + "/plugins")
	if err != nil {
		fatalf("%v", err)
	}
	defer resp.Body.Close()
	var plugins []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&plugins)
	for _, p := range plugins {
		status := "OK"
		if !p["available"].(bool) {
			status = "UNAVAILABLE"
		}
		fmt.Printf("%-32s %s\n", p["name"], status)
	}
}

func cmdEvents(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	host := fs.String("host", defaultBase, "")
	_ = fs.Parse(args)
	resp, err := http.Get(*host + "/events")
	if err != nil {
		fatalf("%v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func pollUntilDone(host, id string) map[string]any {
	for {
		resp, err := http.Get(host + "/jobs/" + id)
		if err != nil {
			fatalf("%v", err)
		}
		var j map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&j)
		resp.Body.Close()
		if s, _ := j["status"].(string); s == "completed" || s == "failed" || s == "cancelled" {
			return j
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

// reorderFlags shuffles argv so every `-flag` (and its value for non-bool
// flags) precedes the positional args. It's a best-effort heuristic —
// flags that expect a value must be listed here so we take the following
// arg with them. Anything not in the list is treated as a bool-style flag.
func reorderFlags(args []string) []string {
	valued := map[string]bool{
		"-outputs": true, "--outputs": true, "-o": true,
		"-recipe": true, "--recipe": true, "-r": true,
		"-tts": true, "--tts": true,
		"-voice": true, "--voice": true,
		"-speed": true, "--speed": true,
		"-layout": true, "--layout": true,
		"-output-dir": true, "--output-dir": true,
		"-tags": true, "--tags": true,
		"-host": true, "--host": true,
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// value-taking flag with separate arg? consume next
			if valued[a] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

func fatalf(f string, args ...any) {
	fmt.Fprintf(os.Stderr, f+"\n", args...)
	os.Exit(1)
}
