package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/openai/openai-go/v3/responses"
	"github.com/samber/lo"
)

type tool struct {
	param   responses.FunctionToolParam
	runFunc func(context.Context, string) string
}

func (t tool) Name() string                        { return t.param.Name }
func (t tool) Param() *responses.FunctionToolParam { return &t.param }

func (t tool) Run(ctx context.Context, arguments string) string {
	return t.runFunc(ctx, arguments)
}

func toolFunc[T any](fn func(context.Context, T) (string, error)) func(context.Context, string) string {
	return func(ctx context.Context, arguments string) string {
		var input T
		if err := json.Unmarshal([]byte(arguments), &input); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		result, err := fn(ctx, input)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		return result
	}
}

type BashInput struct {
	Command string `json:"command"`
}

type ReadFileInput struct {
	Path  string `json:"path"`
	Limit int    `json:"limit"`
}

type WriteFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type EditFileInput struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

type GlobInput struct {
	Pattern string `json:"pattern"`
}

var root = lo.Must(os.OpenRoot("."))

func runBash(ctx context.Context, input BashInput) (string, error) {
	fmt.Print("\n",
		lipgloss.NewStyle().
			Foreground(lipgloss.BrightYellow).
			Render("$ ", input.Command),
	)

	cmd := exec.CommandContext(ctx, "bash", "-c", input.Command)
	output, err := cmd.CombinedOutput()

	result := struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}{
		Output: string(output),
	}

	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return "", err
		}
	}

	fmt.Println("\n",
		lipgloss.NewStyle().
			Foreground(lipgloss.BrightGreen).
			Render(result.Output),
	)

	data, err := json.Marshal(result)
	return string(data), err
}

func runRead(_ context.Context, input ReadFileInput) (string, error) {
	data, err := root.ReadFile(input.Path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")

	if input.Limit > 0 && input.Limit < len(lines) {
		lines = append(
			lines[:input.Limit],
			fmt.Sprintf("... (%d more lines)", len(lines)-input.Limit),
		)
	}

	return strings.Join(lines, "\n"), nil
}

func runWrite(_ context.Context, input WriteFileInput) (string, error) {
	if err := root.MkdirAll(path.Dir(input.Path), 0o755); err != nil {
		return "", err
	}

	file, err := root.OpenFile(input.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.WriteString(file, input.Content); err != nil {
		return "", err
	}

	return fmt.Sprintf("Wrote %d bytes to %s", len(input.Content), input.Path), nil
}

func runEdit(_ context.Context, input EditFileInput) (string, error) {
	data, err := root.ReadFile(input.Path)
	if err != nil {
		return "", err
	}

	text := string(data)

	if !strings.Contains(text, input.OldText) {
		return "", fmt.Errorf("text not found in %s", input.Path)
	}

	text = strings.Replace(text, input.OldText, input.NewText, 1)

	file, err := root.OpenFile(input.Path, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.WriteString(file, text); err != nil {
		return "", err
	}

	return fmt.Sprintf("Edited %s", input.Path), nil
}

func runGlob(_ context.Context, input GlobInput) (string, error) {
	pattern := filepath.ToSlash(input.Pattern)

	matches, err := doublestar.Glob(root.FS(), pattern)
	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "(no matches)", nil
	}

	if len(matches) > 200 {
		matches = append(matches[:200], "... (more matches omitted; narrow the pattern)")
	}

	return strings.Join(matches, "\n"), nil
}
