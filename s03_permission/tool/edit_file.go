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

type EditFile struct{}

func (e EditFile) Name() string { return e.Param().Name }

func (e EditFile) Param() *responses.FunctionToolParam {
	return &responses.FunctionToolParam{
		Name:        "edit_file",
		Description: openai.String("Replace exact text in a file once."),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":     map[string]any{"type": "string"},
				"old_text": map[string]any{"type": "string"},
				"new_text": map[string]any{"type": "string"},
			},
			"required":             []string{"path", "old_text", "new_text"},
			"additionalProperties": false,
		},
		Strict: openai.Bool(true),
	}
}

func (e EditFile) Run(_ context.Context, arguments string) string {
	var input struct {
		Path    string `json:"path"`
		OldText string `json:"old_text"`
		NewText string `json:"new_text"`
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

	text := string(data)
	if !strings.Contains(text, input.OldText) {
		return fmt.Sprintf("Error: text not found in %s", input.Path)
	}
	text = strings.Replace(text, input.OldText, input.NewText, 1)
	if err := os.WriteFile(filePath, []byte(text), 0o644); err != nil {
		return errorResult(err)
	}
	return fmt.Sprintf("Edited %s", input.Path)
}
