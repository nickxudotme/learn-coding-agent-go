package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/samber/lo"
)

const (
	BaseURL = "http://127.0.0.1:11434/v1" // brew install ollama && brew services start ollama
	Model   = "qwen3.5:9b"                // ollama pull qwen3.5:9b
)

func Read() string {
	fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.Cyan).Render("s00 >> "))
	reader := bufio.NewReader(os.Stdin)
	msg := lo.Must(reader.ReadString('\n'))
	return strings.TrimSpace(msg)
}

func main() {
	ctx := context.Background()
	client := openai.NewClient(option.WithBaseURL(BaseURL))

	for input, preRespID := Read(), ""; !lo.Contains([]string{"q", "exit", ""}, input); input = Read() {
		params := responses.ResponseNewParams{
			Model:              Model,
			Reasoning:          openai.ReasoningParam{Effort: openai.ReasoningEffortNone},
			Input:              responses.ResponseNewParamsInputUnion{OfString: openai.String(input)},
			PreviousResponseID: lo.If(lo.IsNotEmpty(preRespID), openai.String(preRespID)).Else(param.Opt[string]{}),
		}
		stream := client.Responses.NewStreaming(ctx, params)
		for stream.Next() {
			event := stream.Current()
			switch event.Type {
			case "response.created":
				preRespID = event.Response.ID
			case "response.output_text.delta":
				fmt.Print(event.Delta)
			}
		}

		//var response *responses.Response
		//for stream.Next() {
		//	event := stream.Current()
		//	switch event.Type {
		//	case "response.output_text.delta":
		//		fmt.Print(event.Delta)
		//	case "response.completed":
		//		response = &event.Response
		//	}
		//}
		lo.Must0(stream.Err())
		fmt.Println()
	}

}

var bashTool = responses.ToolUnionParam{
	OfFunction: &responses.FunctionToolParam{
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
	},
}

func RunBash(command string) (output string, exitCode int) {
	cmd := exec.Command("bash", "-c", command)

	outputBytes, err := cmd.CombinedOutput()
	if err == nil {
		return string(outputBytes), 0
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return string(outputBytes), exitErr.ExitCode()
	}

	panic(err)
}
