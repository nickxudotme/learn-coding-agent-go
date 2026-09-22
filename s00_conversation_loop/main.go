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
	var history []responses.ResponseInputItemUnionParam

	for input := Read(); !lo.Contains([]string{"q", "exit", ""}, input); input = Read() {
		history = append(history, responses.ResponseInputItemParamOfMessage(input, responses.EasyInputMessageRoleUser))
		params := responses.ResponseNewParams{
			Model:     Model,
			Reasoning: openai.ReasoningParam{Effort: openai.ReasoningEffortNone},
			Input:     responses.ResponseNewParamsInputUnion{OfInputItemList: history},
		}
		stream := client.Responses.NewStreaming(ctx, params)
		var response responses.Response
		for stream.Next() {
			event := stream.Current()
			switch event.Type {
			case "response.output_text.delta":
				fmt.Print(event.Delta)
			case "response.completed":
				response = event.Response
			}
		}
		lo.Must0(stream.Err())

		lo.ForEach(response.Output, func(item responses.ResponseOutputItemUnion, _ int) {
			history = append(history, param.Override[responses.ResponseInputItemUnionParam](json.RawMessage(item.RawJSON())))
		})

		fmt.Println()
	}
}
