package parentmap_test

import (
	"context"
	"testing"

	"github.com/nickqiaoo/tadk/agent"
	"github.com/nickqiaoo/tadk/agent/llmagent"
	"github.com/nickqiaoo/tadk/event"
	"github.com/nickqiaoo/tadk/internal/utils"
	"github.com/nickqiaoo/tadk/message"
	"github.com/nickqiaoo/tadk/model"
	"github.com/nickqiaoo/tadk/runner/parentmap"
)

// mockModelAdapter is a minimal mock of model.ModelAdapter.
type mockModelAdapter struct{}

func (m *mockModelAdapter) Name() string { return "mock" }
func (m *mockModelAdapter) Generate(context.Context, *model.Request) (*message.Message, error) {
	return nil, nil
}
func (m *mockModelAdapter) Stream(ctx context.Context, req *model.Request) *model.EventStream {
	return model.NewEventStream(
		func(yield func(event.Event, error) bool) {},
		func() (*message.Message, error) { return nil, nil },
	)
}

func TestNew(t *testing.T) {
	child1_1 := utils.Must(agent.New(agent.Config{
		Name: "child1_1",
	}))

	child1 := utils.Must(agent.New(agent.Config{
		Name:      "child1",
		SubAgents: []agent.Agent{child1_1},
	}))

	child2 := utils.Must(agent.New(agent.Config{
		Name: "child2",
	}))

	root := utils.Must(agent.New(agent.Config{
		Name:      "root",
		SubAgents: []agent.Agent{child1, child2},
	}))

	got, err := parentmap.New(root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify parent relationships
	if got, want := got.ParentOf(child1_1.Name()), child1; got != want {
		t.Errorf("ParentOf(%q) = %v, want %v", child1_1.Name(), got.Name(), want.Name())
	}
	if got, want := got.ParentOf(child1.Name()), root; got != want {
		t.Errorf("ParentOf(%q) = %v, want %v", child1.Name(), got.Name(), want.Name())
	}
	if got, want := got.ParentOf(child2.Name()), root; got != want {
		t.Errorf("ParentOf(%q) = %v, want %v", child2.Name(), got.Name(), want.Name())
	}
	if got := got.ParentOf(root.Name()); got != nil {
		t.Errorf("ParentOf(%q) = %v, want nil", root.Name(), got.Name())
	}

	// Verify children relationships
	rootChildren := got.ChildrenOf(root.Name())
	if len(rootChildren) != 2 || rootChildren[0] != child1 || rootChildren[1] != child2 {
		t.Errorf("ChildrenOf(%q) wrong children", root.Name())
	}
	child1Children := got.ChildrenOf(child1.Name())
	if len(child1Children) != 1 || child1Children[0] != child1_1 {
		t.Errorf("ChildrenOf(%q) wrong children", child1.Name())
	}
	if len(got.ChildrenOf(child1_1.Name())) != 0 {
		t.Errorf("ChildrenOf(%q) = %v, want empty", child1_1.Name(), got.ChildrenOf(child1_1.Name()))
	}
	if len(got.ChildrenOf(child2.Name())) != 0 {
		t.Errorf("ChildrenOf(%q) = %v, want empty", child2.Name(), got.ChildrenOf(child2.Name()))
	}

	// Verify root agent
	if got, want := got.RootAgent(child1_1), root; got != want {
		t.Errorf("RootAgent(%v) = %v, want %v", child1_1.Name(), got.Name(), want.Name())
	}

	// Verify FindByName
	for _, want := range []agent.Agent{root, child1, child1_1, child2} {
		if g := got.FindByName(want.Name()); g != want {
			t.Errorf("FindByName(%q) = %v, want %v", want.Name(), g, want)
		}
	}
	if g := got.FindByName("missing"); g != nil {
		t.Errorf("FindByName(missing) = %v, want nil", g)
	}
}

func TestMap_RootAgent(t *testing.T) {
	model := &mockModelAdapter{}

	nonLLM := utils.Must(agent.New(agent.Config{
		Name: "mock",
	}))
	b := utils.Must(llmagent.New(llmagent.Config{
		Name:      "b",
		Model:     model,
		SubAgents: []agent.Agent{nonLLM},
	}))
	a := utils.Must(llmagent.New(llmagent.Config{
		Name:      "a",
		Model:     model,
		SubAgents: []agent.Agent{b},
	}))
	root := utils.Must(llmagent.New(llmagent.Config{
		Name:      "root",
		Model:     model,
		SubAgents: []agent.Agent{a},
	}))

	agentName := func(a agent.Agent) string {
		if a == nil {
			return "nil"
		}
		return a.Name()
	}

	parents, err := parentmap.New(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		agent agent.Agent
		want  agent.Agent
	}{
		{root, root},
		{a, root},
		{b, root},
		{nonLLM, root},
		{nil, nil},
	} {
		t.Run("agent="+agentName(tc.agent), func(t *testing.T) {
			gotRoot := parents.RootAgent(tc.agent)
			if got, want := agentName(gotRoot), agentName(tc.want); got != want {
				t.Errorf("rootAgent(%q) = %q, want %q", agentName(tc.agent), got, want)
			}
		})
	}
}
