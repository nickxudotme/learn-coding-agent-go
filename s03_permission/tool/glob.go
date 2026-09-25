package tool

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

type Glob struct{}

func (g Glob) Name() string { return g.Param().Name }

func (g Glob) Param() *responses.FunctionToolParam {
	return &responses.FunctionToolParam{
		Name:        "glob",
		Description: openai.String("Find files matching a glob pattern."),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
			},
			"required":             []string{"pattern"},
			"additionalProperties": false,
		},
		Strict: openai.Bool(true),
	}
}

func (g Glob) Run(_ context.Context, arguments string) string {
	var input struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return errorResult(err)
	}
	fmt.Print("\n", lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Render(input.Pattern), "\n")

	pattern := filepath.ToSlash(strings.TrimPrefix(input.Pattern, "./"))
	if pattern == "" {
		return "Error: pattern is empty"
	}
	if filepath.IsAbs(input.Pattern) {
		return "Error: absolute patterns are not allowed"
	}
	if slices.Contains(strings.Split(pattern, "/"), "..") {
		return fmt.Sprintf("Error: pattern escapes workspace: %s", input.Pattern)
	}

	root, err := workspaceRoot()
	if err != nil {
		return errorResult(err)
	}
	var matches []string
	err = filepath.WalkDir(root, func(filePath string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == root {
			return nil
		}
		relativePath, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if matchGlob(pattern, relativePath) {
			matches = append(matches, relativePath)
		}
		return nil
	})
	if err != nil {
		return errorResult(err)
	}

	slices.Sort(matches)
	if len(matches) == 0 {
		return "(no matches)"
	}
	if len(matches) > 200 {
		matches = append(matches[:200], "... (more matches omitted; narrow the pattern)")
	}
	return strings.Join(matches, "\n")
}
