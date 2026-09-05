package mcpsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

// fetchKokoroVoices duplicates the httpapi helper so mcpsvc doesn't depend
// on httpapi. It's a tiny bit of code and the two callers speak different
// output formats anyway.
func fetchKokoroVoices(ctx context.Context) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8770/voices", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("kokoro %d: %s", resp.StatusCode, string(body))
	}
	var payload struct {
		Voices []string `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Voices, nil
}

func macSayVoices(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "say", "-v", "?").Output()
	if err != nil {
		return nil, err
	}
	var voices []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) > 0 {
			voices = append(voices, fields[0])
		}
	}
	return voices, nil
}
