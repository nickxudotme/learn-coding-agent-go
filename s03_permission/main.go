package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/samber/lo"
	"learn-coding-agent-go/s03_permission/tool"
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
	fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.BrightCyan).Render("s03 >> "))
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
			Tools: lo.Map(tool.List, func(item tool.Tool, _ int) responses.ToolUnionParam {
				return responses.ToolUnionParam{OfFunction: item.Param()}
			}),
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
			t, ok := tool.Map[call.Name]
			output := fmt.Sprintf("Unknown tool: %s", call.Name)
			if ok {
				fmt.Print("\n", lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Render(call.Name))
				output = t.Run(ctx, call.Arguments)
			}

			toolResults = append(toolResults,
				responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: openai.String(call.CallID),
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
							OfString: openai.String(output),
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
