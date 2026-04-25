// Package functiontool provides a tool that wraps a Go function.
package functiontool

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nickqiaoo/tadk/internal/typeutil"
	"github.com/nickqiaoo/tadk/tool"
)

// Config is the input to New.
type Config struct {
	// The name of this tool.
	Name string
	// A human-readable description of the tool.
	Description string
	// An optional JSON schema object defining the expected parameters for the tool.
	// If it is nil, the tool tries to infer the schema based on the handler type.
	InputSchema *jsonschema.Schema
	// An optional JSON schema object defining the structure of the tool's output.
	// If it is nil, the tool tries to infer the schema based on the handler type.
	OutputSchema *jsonschema.Schema
	// IsLongRunning marks this tool as a long-running operation.
	IsLongRunning bool
}

// Func represents a Go function that can be wrapped in a tool.
// It takes a tool.Context and a generic argument type, and returns a generic
// result type, optional flow-control signals, and an error.
type Func[TArgs, TResults any] func(tool.Context, TArgs) (TResults, *tool.Control, error)

// ErrInvalidArgument indicates the input parameter type is invalid.
var ErrInvalidArgument = errors.New("invalid argument")

// New creates a new tool with a name, description, and the provided handler.
// Input schema is automatically inferred from the input and output types.
func New[TArgs, TResults any](cfg Config, handler Func[TArgs, TResults]) (tool.Tool, error) {
	var zeroArgs TArgs
	argsType := reflect.TypeOf(zeroArgs)
	for argsType != nil && argsType.Kind() == reflect.Ptr {
		argsType = argsType.Elem()
	}
	if argsType == nil || (argsType.Kind() != reflect.Struct && argsType.Kind() != reflect.Map) {
		return nil, fmt.Errorf("input must be a struct or a map or a pointer to those types, but received: %v: %w", argsType, ErrInvalidArgument)
	}

	ischema, err := resolvedSchema[TArgs](cfg.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to infer input schema: %w", err)
	}
	oschema, err := resolvedSchema[TResults](cfg.OutputSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to infer output schema: %w", err)
	}

	return &functionTool[TArgs, TResults]{
		cfg:          cfg,
		inputSchema:  ischema,
		outputSchema: oschema,
		handler:      handler,
	}, nil
}

// functionTool wraps a Go function.
type functionTool[TArgs, TResults any] struct {
	cfg Config

	// A JSON Schema object defining the expected parameters for the tool.
	inputSchema *jsonschema.Resolved
	// A JSON Schema object defining the result of the tool.
	outputSchema *jsonschema.Resolved

	// handler is the Go function.
	handler Func[TArgs, TResults]
}

// Description implements tool.Tool.
func (f *functionTool[TArgs, TResults]) Description() string {
	return f.cfg.Description
}

// Name implements tool.Tool.
func (f *functionTool[TArgs, TResults]) Name() string {
	return f.cfg.Name
}

func (f *functionTool[TArgs, TResults]) Schema() map[string]any {
	if f.inputSchema == nil {
		return nil
	}
	return schemaAnyToMap(f.inputSchema.Schema())
}

// Execute runs the unified public tool interface.
func (f *functionTool[TArgs, TResults]) Execute(ctx tool.Context, args map[string]any) (result any, ctrl *tool.Control, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in tool %q: %v\nstack: %s", f.Name(), r, debug.Stack())
		}
	}()

	input, err := typeutil.ConvertToWithJSONSchema[map[string]any, TArgs](args, f.inputSchema)
	if err != nil {
		return nil, nil, err
	}

	output, ctrl, err := f.handler(ctx, input)
	if err != nil {
		return nil, ctrl, err
	}
	resp, cErr := typeutil.ConvertToWithJSONSchema[TResults, map[string]any](output, f.outputSchema)
	if cErr == nil { // all good
		return resp, ctrl, nil
	}

	// Specs requires the result to be a map (dict in python). python impl allows basic types when building response event
	// functions.py __build_response_event does the following
	// if not isinstance(function_result, dict):
	// 		function_result = {'result': function_result}
	if f.outputSchema != nil {
		if err1 := f.outputSchema.Validate(output); err1 != nil {
			return resp, ctrl, cErr // if it fails propagate original err.
		}
	}
	return map[string]any{"result": output}, ctrl, nil
}

func resolvedSchema[T any](override *jsonschema.Schema) (*jsonschema.Resolved, error) {
	if override != nil {
		return override.Resolve(nil)
	}
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		return nil, err
	}
	return schema.Resolve(nil)
}

func schemaAnyToMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}
