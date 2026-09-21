package main

import (
	"bufio"
	"context"
	"encoding/json"
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

var (
	client  = openai.NewClient(option.WithBaseURL(BaseURL))
	history responses.ResponseInputParam
)

func Read() string {
	fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.BrightCyan).Render("s01 >> "))
	reader := bufio.NewReader(os.Stdin)
	msg := lo.Must(reader.ReadString('\n'))
	return strings.TrimSpace(msg)
}

func main() {
	ctx := context.Background()

	for input := Read(); !lo.Contains([]string{"q", "exit", ""}, input); input = Read() {
		AgentLoop(ctx, input)
		fmt.Println()
	}
}

func AgentLoop(ctx context.Context, input string) {
	history = append(history, responses.ResponseInputItemParamOfMessage(input, responses.EasyInputMessageRoleUser))

	// Agent 可能要连续多次调用工具
	for {
		params := responses.ResponseNewParams{
			Model:     Model,
			Reasoning: openai.ReasoningParam{Effort: openai.ReasoningEffortNone},
			Input:     responses.ResponseNewParamsInputUnion{OfInputItemList: history},
			Tools:     []responses.ToolUnionParam{bashTool},
		}
		stream := client.Responses.NewStreaming(ctx, params)
		var response responses.Response
		for stream.Next() {
			event := stream.Current()
			switch event.Type {
			case "response.output_text.delta":
				fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.White).Render(event.Delta))
			case "response.completed":
				response = event.Response
			}
		}
		lo.Must0(stream.Err())

		lo.ForEach(response.Output, func(item responses.ResponseOutputItemUnion, _ int) {
			history = append(history, param.Override[responses.ResponseInputItemUnionParam](json.RawMessage(item.RawJSON())))
		})

		// 收集这一轮的所有 tool call
		var toolResults []responses.ResponseInputItemUnionParam
		for _, item := range response.Output {
			if item.Type != "function_call" {
				continue
			}

			call := item.AsFunctionCall()
			if call.Name != "bash" {
				continue
			}

			var input BashInput
			lo.Must0(json.Unmarshal([]byte(call.Arguments), &input))
			fmt.Print("\n", lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Render("$ ", input.Command))
			result := RunBash(input)
			fmt.Println("\n", lipgloss.NewStyle().Foreground(lipgloss.BrightGreen).Render(result.Output))

			toolResults = append(toolResults,
				responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: openai.String(call.CallID),
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
							OfString: openai.String(string(lo.Must(json.Marshal(result)))),
						},
					},
				},
			)
		}
		// 没有 tool call：
		// 相当于 Anthropic 的 stop_reason != "tool_use"
		if len(toolResults) == 0 {
			return
		}
		history = append(history, toolResults...)
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

type BashInput struct {
	Command string `json:"command"`
}

type BashResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

func RunBash(input BashInput) BashResult {
	cmd := exec.Command("bash", "-c", input.Command)
	outputBytes, err := cmd.CombinedOutput()
	if err == nil {
		return BashResult{Output: string(outputBytes), ExitCode: 0}
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return BashResult{Output: string(outputBytes), ExitCode: exitErr.ExitCode()}
	}
	panic(err)
}
