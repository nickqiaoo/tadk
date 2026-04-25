// Package exitlooptool provides a tool that allows an agent to exit a loop.
package exitlooptool

import (
	"fmt"

	"github.com/nickqiaoo/tadk/tool"
	"github.com/nickqiaoo/tadk/tool/functiontool"
)

// EmptyArgs is an empty struct used as an argument for the exitLoop tool.
type EmptyArgs struct{}

func exitLoop(ctx tool.Context, myArgs EmptyArgs) (map[string]string, *tool.Control, error) {
	return map[string]string{}, &tool.Control{ExitToParent: true, Terminal: true}, nil
}

// New creates an instance of an exitLoop tool.
func New() (tool.Tool, error) {
	exitLoopTool, err := functiontool.New(functiontool.Config{
		Name:        "exit_loop",
		Description: "Exits the loop.\nCall this function only when you are instructed to do so.\n",
	}, exitLoop)
	if err != nil {
		return nil, fmt.Errorf("error creating exit loop tool: %w", err)
	}
	return exitLoopTool, nil
}
