package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Model is one row of `pi --list-models`.
type Model struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// ParseModelList parses `pi --list-models` output: a whitespace-aligned table
// whose header starts with "provider  model …". Columns 1 and 2 of every data
// row are taken; blank lines and the header are skipped.
func ParseModelList(out string) []Model {
	var models []Model
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.EqualFold(fields[0], "provider") && strings.EqualFold(fields[1], "model") {
			continue
		}
		models = append(models, Model{Provider: fields[0], ID: fields[1]})
	}
	return models
}

// ListModels runs `<bin> --list-models` with a timeout and parses it.
func ListModels(bin string, timeout time.Duration) ([]Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--list-models")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s --list-models: %s", filepath.Base(bin), msg)
	}
	return ParseModelList(string(out)), nil
}

// DefaultModel returns the `defaultModel` of ~/.pi/agent/settings.json, or ""
// when the file is missing or unreadable.
func DefaultModel(userHome string) string {
	data, err := os.ReadFile(filepath.Join(userHome, ".pi", "agent", "settings.json"))
	if err != nil {
		return ""
	}
	var s struct {
		DefaultModel string `json:"defaultModel"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return ""
	}
	return s.DefaultModel
}
