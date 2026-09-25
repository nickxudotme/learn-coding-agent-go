package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3/responses"
	"github.com/samber/lo"
)

type Tool interface {
	Name() string
	Param() *responses.FunctionToolParam
	Run(ctx context.Context, arguments string) string
}

var List = []Tool{Bash{}, Glob{}, EditFile{}, ReadFile{}, WriteFile{}}

var Map = lo.SliceToMap(List, func(item Tool) (string, Tool) { return item.Name(), item })

func errorResult(err error) string { return fmt.Sprintf("Error: %v", err) }

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
