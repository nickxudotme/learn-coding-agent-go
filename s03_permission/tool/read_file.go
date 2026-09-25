package tool

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

type ReadFile struct{}

func (r ReadFile) Name() string { return r.Param().Name }

func (r ReadFile) Param() *responses.FunctionToolParam {
	return &responses.FunctionToolParam{
		Name:        "read_file",
		Description: openai.String("Read file contents."),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
	}
}

func (r ReadFile) Run(_ context.Context, arguments string) string {
	var input struct {
		Path  string `json:"path"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return errorResult(err)
	}
	fmt.Print("\n", lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Render(input.Path), "\n")

	filePath, err := safePath(input.Path)
	if err != nil {
		return errorResult(err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return errorResult(err)
	}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if text == "" {
		return ""
	}
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	if input.Limit > 0 && input.Limit < len(lines) {
		lines = append(lines[:input.Limit], fmt.Sprintf("... (%d more lines)", len(lines)-input.Limit))
	}
	return strings.Join(lines, "\n")
}
