package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Execute runs the plugin against ctx. On success, the artifact is placed at
// ctx.Output and Execute returns that path (typically the same as ctx.Output,
// but plugins that write elsewhere via Produces get moved into place here).
func Execute(runctx context.Context, m *Manifest, tctx *TemplateContext) (string, error) {
	timeout := time.Duration(m.Run.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	runctx, cancel := context.WithTimeout(runctx, timeout)
	defer cancel()

	switch m.Run.Kind {
	case "command":
		return execCommand(runctx, m, tctx)
	case "http":
		return execHTTP(runctx, m, tctx)
	default:
		return "", Errorf(m, "unknown run.kind %q", m.Run.Kind)
	}
}

func execCommand(ctx context.Context, m *Manifest, tctx *TemplateContext) (string, error) {
	if len(m.Run.Argv) == 0 {
		return "", Errorf(m, "command plugin has empty argv")
	}
	argv := make([]string, len(m.Run.Argv))
	for i, a := range m.Run.Argv {
		argv[i] = Expand(a, tctx)
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)

	if m.Run.Workdir != "" {
		cmd.Dir = Expand(m.Run.Workdir, tctx)
	}
	if len(m.Run.Env) > 0 {
		env := os.Environ()
		for k, v := range m.Run.Env {
			env = append(env, k+"="+Expand(v, tctx))
		}
		cmd.Env = env
	}
	if m.Run.Stdin != "" {
		cmd.Stdin = strings.NewReader(Expand(m.Run.Stdin, tctx))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", Errorf(m, "%s: %w (stderr: %s)", argv[0], err, strings.TrimSpace(stderr.String()))
	}

	// Where's the output?
	//   output_from: stdout   -> capture stdout, write to tctx.Output
	//   produces:    template -> tool wrote there, move/copy to tctx.Output
	//   (default)             -> tool wrote directly to tctx.Output
	switch m.Run.OutputFrom {
	case "stdout":
		if err := os.WriteFile(tctx.Output, stdout.Bytes(), 0o644); err != nil {
			return "", Errorf(m, "write stdout to %s: %w", tctx.Output, err)
		}
	case "", "file":
		if m.Run.Produces != "" {
			produced := Expand(m.Run.Produces, tctx)
			if produced != tctx.Output {
				if err := os.Rename(produced, tctx.Output); err != nil {
					// Cross-device or busy — fall back to copy.
					if err := copyFile(produced, tctx.Output); err != nil {
						return "", Errorf(m, "relocate %s -> %s: %w", produced, tctx.Output, err)
					}
					_ = os.Remove(produced)
				}
			}
		}
	default:
		return "", Errorf(m, "unknown output_from %q", m.Run.OutputFrom)
	}

	if _, err := os.Stat(tctx.Output); err != nil {
		return "", Errorf(m, "no artifact at %s after run: %w", tctx.Output, err)
	}
	return tctx.Output, nil
}

func execHTTP(ctx context.Context, m *Manifest, tctx *TemplateContext) (string, error) {
	url := Expand(m.Run.URL, tctx)
	method := strings.ToUpper(m.Run.Method)
	if method == "" {
		method = "POST"
	}

	var body io.Reader
	if len(m.Run.BodyJSON) > 0 {
		expanded := ExpandJSON(m.Run.BodyJSON, tctx)
		b, err := json.Marshal(expanded)
		if err != nil {
			return "", Errorf(m, "encode body: %w", err)
		}
		body = bytes.NewReader(b)
	} else if m.Run.BodyTemplate != "" {
		body = strings.NewReader(Expand(m.Run.BodyTemplate, tctx))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "", Errorf(m, "build request: %w", err)
	}
	if len(m.Run.BodyJSON) > 0 && m.Run.Headers["Content-Type"] == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range m.Run.Headers {
		req.Header.Set(k, Expand(v, tctx))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", Errorf(m, "%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", Errorf(m, "%s %s -> %d: %s", method, url, resp.StatusCode, string(snippet))
	}

	dest := Expand(m.Run.ResponseToFile, tctx)
	if dest == "" {
		dest = tctx.Output
	}
	out, err := os.Create(dest)
	if err != nil {
		return "", Errorf(m, "create %s: %w", dest, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", Errorf(m, "write %s: %w", dest, err)
	}
	return dest, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// unused imports guard
var _ = fmt.Sprint
