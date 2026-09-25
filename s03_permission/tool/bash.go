package tool

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os/exec"

	"charm.land/lipgloss/v2"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/samber/lo"
)

type Bash struct{}

func (b Bash) Name() string { return b.Param().Name }

func (b Bash) Param() *responses.FunctionToolParam {
	return &responses.FunctionToolParam{
		Name:        "bash",
		Description: openai.String("Run a shell command."),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "The command to run."},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
		Strict: openai.Bool(true),
	}
}

func (b Bash) Run(ctx context.Context, arguments string) string {
	var input struct {
		Command string `json:"command"`
	}
	var result struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	lo.Must0(json.Unmarshal([]byte(arguments), &input))
	fmt.Print("\n", lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Render("$ ", input.Command))
	cmd := exec.CommandContext(ctx, "bash", "-c", input.Command)
	outputBytes, err := cmd.CombinedOutput()

	if err == nil {
		result.Output = string(outputBytes)
		result.ExitCode = 0
	} else if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		result.Output = string(outputBytes)
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.Output = fmt.Sprintf("[ERROR] %v", err)
		result.ExitCode = -1
	}
	fmt.Println("\n", lipgloss.NewStyle().Foreground(lipgloss.BrightGreen).Render(result.Output))
	return string(lo.Must(json.Marshal(result)))
}
