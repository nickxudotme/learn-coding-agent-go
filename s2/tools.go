package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/samber/lo"
)

type tool struct {
	param   responses.FunctionToolParam
	runFunc func(context.Context, string) string
}

func (t tool) Name() string                                     { return t.param.Name }
func (t tool) Param() *responses.FunctionToolParam              { return &t.param }
func (t tool) Run(ctx context.Context, arguments string) string { return t.runFunc(ctx, arguments) }

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

var ToolList = []tool{
	{
		param: responses.FunctionToolParam{
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
		runFunc: runBash,
	},
	{
		param: responses.FunctionToolParam{
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
		},
		runFunc: runRead,
	},
	{
		param: responses.FunctionToolParam{
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
		},
		runFunc: runWrite,
	},
	{
		param: responses.FunctionToolParam{
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
		},
		runFunc: runEdit,
	},
	{
		param: responses.FunctionToolParam{
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
		},
		runFunc: runGlob,
	},
}

var ToolMap = lo.SliceToMap(ToolList, func(item tool) (string, tool) { return item.Name(), item })

func runBash(ctx context.Context, arguments string) string {
	var input struct {
		Command string `json:"command"`
	}
	var result struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	lo.Must0(json.Unmarshal([]byte(arguments), &input))

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

	return string(lo.Must(json.Marshal(result)))
}

func runRead(_ context.Context, arguments string) string {
	input, err := decodeArguments[ReadFileInput](arguments)
	if err != nil {
		return errorResult(err)
	}

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

func runWrite(_ context.Context, arguments string) string {
	input, err := decodeArguments[WriteFileInput](arguments)
	if err != nil {
		return errorResult(err)
	}

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

func runEdit(_ context.Context, arguments string) string {
	input, err := decodeArguments[EditFileInput](arguments)
	if err != nil {
		return errorResult(err)
	}

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

func runGlob(_ context.Context, arguments string) string {
	input, err := decodeArguments[GlobInput](arguments)
	if err != nil {
		return errorResult(err)
	}

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

func decodeArguments[T any](arguments string) (T, error) {
	var input T
	err := json.Unmarshal([]byte(arguments), &input)
	return input, err
}

func errorResult(err error) string {
	return fmt.Sprintf("Error: %v", err)
}

func safePath(name string) (string, error) {
	root, err := workspaceRoot()
	if err != nil {
		return "", err
	}

	filePath := name
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(root, filePath)
	}
	filePath, err = filepath.Abs(filePath)
	if err != nil {
		return "", err
	}
	if !isWithin(root, filePath) {
		return "", fmt.Errorf("path escapes workspace: %s", name)
	}

	resolvedPath, err := resolveExistingPrefix(filePath)
	if err != nil {
		return "", err
	}
	if !isWithin(root, resolvedPath) {
		return "", fmt.Errorf("path escapes workspace: %s", name)
	}
	return resolvedPath, nil
}

func workspaceRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(root)
}

func isWithin(root, filePath string) bool {
	relativePath, err := filepath.Rel(root, filePath)
	if err != nil {
		return false
	}
	return relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

func resolveExistingPrefix(filePath string) (string, error) {
	currentPath := filePath
	var suffix []string

	for {
		_, err := os.Lstat(currentPath)
		if err == nil {
			resolvedPath, err := filepath.EvalSymlinks(currentPath)
			if err != nil {
				return "", err
			}
			for _, s := range slices.Backward(suffix) {
				resolvedPath = filepath.Join(resolvedPath, s)
			}
			return filepath.Clean(resolvedPath), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return filePath, nil
		}
		suffix = append(suffix, filepath.Base(currentPath))
		currentPath = parentPath
	}
}

func matchGlob(pattern, name string) bool {
	return matchGlobParts(splitPath(pattern), splitPath(name))
}

func matchGlobParts(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		if matchGlobParts(pattern[1:], name) {
			return true
		}
		return len(name) > 0 && matchGlobParts(pattern, name[1:])
	}
	if len(name) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], name[0])
	return err == nil && matched && matchGlobParts(pattern[1:], name[1:])
}

func splitPath(value string) []string {
	value = strings.Trim(value, "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}
