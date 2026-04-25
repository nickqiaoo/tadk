package app

import (
	"strings"

	"github.com/nickqiaoo/tadk/cmd/tui/runtime"
)

type slashCommand struct {
	name string
	args []string
}

func parseSlashCommand(input string) (slashCommand, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return slashCommand{}, false
	}
	fields := strings.Fields(strings.TrimPrefix(input, "/"))
	if len(fields) == 0 {
		return slashCommand{}, false
	}
	return slashCommand{
		name: strings.ToLower(fields[0]),
		args: fields[1:],
	}, true
}

func parseModelArgs(args []string) runtime.ModelChoice {
	if len(args) == 0 {
		return runtime.ModelChoice{}
	}
	if len(args) == 1 && strings.Contains(args[0], ":") {
		parts := strings.SplitN(args[0], ":", 2)
		return runtime.ModelChoice{Provider: parts[0], Name: parts[1]}
	}
	choice := runtime.ModelChoice{Provider: args[0]}
	if len(args) > 1 {
		choice.Name = strings.Join(args[1:], " ")
	}
	return choice
}
