package tool

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

type WriteFile struct{}

func (w WriteFile) Name() string { return w.Param().Name }

func (w WriteFile) Param() *responses.FunctionToolParam {
	return &responses.FunctionToolParam{
		Name:        "write_file",
		Description: openai.String("Write content to a file."),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Strict: openai.Bool(true),
	}
}

func (w WriteFile) Run(_ context.Context, arguments string) string {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return errorResult(err)
	}
	fmt.Print("\n", lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Render(input.Path), "\n")

	filePath, err := safePath(input.Path)
	if err != nil {
		return errorResult(err)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return errorResult(err)
	}
	if err := os.WriteFile(filePath, []byte(input.Content), 0o644); err != nil {
		return errorResult(err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(input.Content), input.Path)
}
