package llminternal

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/internal/toolinternal"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/tool"
)

type mockFunctionTool struct {
	name    string
	runFunc func(tool.Context, map[string]any) (map[string]any, *tool.Control, error)
}

func (m *mockFunctionTool) Name() string {
	return m.name
}

func (m *mockFunctionTool) Description() string {
	return "mock tool"
}

func (m *mockFunctionTool) Schema() map[string]any {
	return nil
}

func (m *mockFunctionTool) Execute(ctx tool.Context, args map[string]any) (any, *tool.Control, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, args)
	}
	return nil, nil, nil
}

type testCase struct {
	name       string
	tool       tool.Tool
	args       map[string]any
	extensions []agent.Extension
	want       map[string]any
}

type toolHookExtension struct {
	agent.DefaultExtension
	before func(agent.InvocationContext, *message.ToolCall) (*message.ToolResult, error)
	after  func(agent.InvocationContext, *message.ToolCall, *message.ToolResult, error) (*message.ToolResult, error)
}

func (e *toolHookExtension) Name() string { return "tool-hook-extension" }

func (e *toolHookExtension) BeforeToolCall(ctx agent.InvocationContext, call *message.ToolCall) (*message.ToolResult, error) {
	if e.before == nil {
		return nil, nil
	}
	return e.before(ctx, call)
}

func (e *toolHookExtension) AfterToolCall(ctx agent.InvocationContext, call *message.ToolCall, result *message.ToolResult, toolErr error) (*message.ToolResult, error) {
	if e.after == nil {
		return nil, nil
	}
	return e.after(ctx, call, result, toolErr)
}

func TestCallTool(t *testing.T) {
	testCases := []testCase{
		{
			name: "tool runs successfully",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					return map[string]any{"result": "success"}, nil, nil
				},
			},
			args: map[string]any{"key": "value"},
			want: map[string]any{"result": "success"},
		},
		{
			name: "tool error",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					return nil, nil, errors.New("tool error")
				},
			},
			args: map[string]any{"key": "value"},
			want: map[string]any{"error": "tool error"},
		},
		{
			name: "before extension returns result",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					t.Error("tool should not be called")
					return nil, nil, nil
				},
			},
			extensions: []agent.Extension{&toolHookExtension{
				before: func(ctx agent.InvocationContext, call *message.ToolCall) (*message.ToolResult, error) {
					return &message.ToolResult{Content: map[string]any{"result": "intercepted"}}, nil
				},
			}},
			want: map[string]any{"result": "intercepted"},
		},
		{
			name: "before extension returns error",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					t.Error("tool should not be called")
					return nil, nil, nil
				},
			},
			extensions: []agent.Extension{&toolHookExtension{
				before: func(ctx agent.InvocationContext, call *message.ToolCall) (*message.ToolResult, error) {
					return nil, errors.New("before extension error")
				},
			}},
			want: map[string]any{"error": `extension "tool-hook-extension" before tool call failed: before extension error`},
		},
		{
			name: "after extension modifies result",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					return map[string]any{"result": "original"}, nil, nil
				},
			},
			extensions: []agent.Extension{&toolHookExtension{
				after: func(ctx agent.InvocationContext, call *message.ToolCall, result *message.ToolResult, toolErr error) (*message.ToolResult, error) {
					return &message.ToolResult{Content: map[string]any{"result": "modified"}}, nil
				},
			}},
			want: map[string]any{"result": "modified"},
		},
		{
			name: "after extension handles error",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					return nil, nil, errors.New("tool error")
				},
			},
			extensions: []agent.Extension{&toolHookExtension{
				after: func(ctx agent.InvocationContext, call *message.ToolCall, result *message.ToolResult, toolErr error) (*message.ToolResult, error) {
					if toolErr == nil || toolErr.Error() != "tool error" {
						t.Fatalf("unexpected tool error: %v", toolErr)
					}
					return &message.ToolResult{Content: map[string]any{"result": "error handled"}}, nil
				},
			}},
			want: map[string]any{"result": "error handled"},
		},
		{
			name: "after extension returns error",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					return map[string]any{"result": "success"}, nil, nil
				},
			},
			extensions: []agent.Extension{&toolHookExtension{
				after: func(ctx agent.InvocationContext, call *message.ToolCall, result *message.ToolResult, toolErr error) (*message.ToolResult, error) {
					return nil, errors.New("after extension error")
				},
			}},
			want: map[string]any{"error": `extension "tool-hook-extension" after tool call failed: after extension error`},
		},
		{
			name: "no-op extension returns func results",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					return map[string]any{"result": "success"}, nil, nil
				},
			},
			extensions: []agent.Extension{&toolHookExtension{}},
			want:       map[string]any{"result": "success"},
		},
		{
			name: "before extension result passed to after extension",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					t.Error("tool should not be called")
					return nil, nil, nil
				},
			},
			extensions: []agent.Extension{&toolHookExtension{
				before: func(ctx agent.InvocationContext, call *message.ToolCall) (*message.ToolResult, error) {
					return &message.ToolResult{Content: map[string]any{"result": "from_before"}}, nil
				},
				after: func(ctx agent.InvocationContext, call *message.ToolCall, result *message.ToolResult, toolErr error) (*message.ToolResult, error) {
					content, _ := result.Content.(map[string]any)
					if val, ok := content["result"]; !ok || val != "from_before" {
						return nil, errors.New("unexpected result in after extension")
					}
					return &message.ToolResult{Content: map[string]any{"result": "from_after"}}, nil
				},
			}},
			want: map[string]any{"result": "from_after"},
		},
		{
			name: "before extension error passed to after extension",
			tool: &mockFunctionTool{
				name: "testTool",
				runFunc: func(ctx tool.Context, args map[string]any) (map[string]any, *tool.Control, error) {
					t.Error("tool should not be called")
					return nil, nil, nil
				},
			},
			extensions: []agent.Extension{&toolHookExtension{
				before: func(ctx agent.InvocationContext, call *message.ToolCall) (*message.ToolResult, error) {
					return nil, errors.New("error_from_before")
				},
				after: func(ctx agent.InvocationContext, call *message.ToolCall, result *message.ToolResult, toolErr error) (*message.ToolResult, error) {
					if toolErr == nil || toolErr.Error() != `extension "tool-hook-extension" before tool call failed: error_from_before` {
						return nil, errors.New("unexpected error in after extension")
					}
					return &message.ToolResult{Content: map[string]any{"result": "error_handled_in_after"}}, nil
				},
			}},
			want: map[string]any{"result": "error_handled_in_after"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &Flow{
				ToolExts: agent.AsToolExtensions(tc.extensions),
			}
			ctx := agent.NewInvocationContext(t.Context(), agent.InvocationContextParams{
				RunConfig: &agent.RunConfig{Extensions: tc.extensions},
			})
			toolCtx := toolinternal.NewToolContext(ctx, "call-1")
			got, _, err := f.callTool(toolCtx, tc.tool, &message.ToolCall{
				ID:   "call-1",
				Name: tc.tool.Name(),
				Args: tc.args,
			})
			if err != nil {
				got = map[string]any{"error": err.Error()}
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("callTool() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func mergeTerminal(base, other bool) bool {
	return base || other
}

func TestMergeTerminal(t *testing.T) {
	tests := []struct {
		name  string
		base  bool
		other bool
		want  bool
	}{
		{
			name:  "both nil",
			base:  false,
			other: false,
			want:  false,
		},
		{
			name:  "other nil returns base",
			base:  true,
			other: false,
			want:  true,
		},
		{
			name:  "base nil returns other",
			base:  false,
			other: true,
			want:  true,
		},
		{
			name:  "terminal merging - any true wins",
			base:  false,
			other: true,
			want:  true,
		},
		{
			name:  "all fields merged correctly",
			base:  false,
			other: true,
			want:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeTerminal(tc.base, tc.other)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mergeTerminal() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
