package agent

import (
	"iter"
	"testing"

	"github.com/nickqiaoo/tadk/event"
)

func TestAgentTransferability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		disallowTransferParent bool
		wantTransferable       bool
	}{
		{
			name:                   "transfer allowed",
			disallowTransferParent: false,
			wantTransferable:       true,
		},
		{
			name:                   "transfer disallowed",
			disallowTransferParent: true,
			wantTransferable:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parent := &mockTransferableAgent{
				name:                   "parent",
				disallowTransferParent: tt.disallowTransferParent,
			}
			child := &mockTransferableAgent{
				name:   "child",
				parent: parent,
			}

			got := isTransferableAcrossAgentTree(child)
			if got != tt.wantTransferable {
				t.Errorf("isTransferableAcrossAgentTree() = %v, want %v", got, tt.wantTransferable)
			}
		})
	}
}

// mockTransferableAgent is a mock agent for transferability testing.
type mockTransferableAgent struct {
	name                   string
	parent                 *mockTransferableAgent
	disallowTransferParent bool
}

func (m *mockTransferableAgent) Name() string        { return m.name }
func (m *mockTransferableAgent) Description() string { return "" }
func (m *mockTransferableAgent) SubAgents() []Agent  { return nil }
func (m *mockTransferableAgent) Run(ctx InvocationContext) iter.Seq2[event.Event, error] {
	return nil
}

func isTransferableAcrossAgentTree(agentToRun *mockTransferableAgent) bool {
	for cur := agentToRun; cur != nil; cur = cur.parent {
		if cur.disallowTransferParent {
			return false
		}
	}
	return true
}
