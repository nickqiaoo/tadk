package parentmap

import (
	"fmt"

	"github.com/nickqiaoo/tadk/agent"
)

// Tree represents the parent-child relationships in an agent tree.
type Tree struct {
	root     agent.Agent
	parent   map[string]agent.Agent
	children map[string][]agent.Agent
	byName   map[string]agent.Agent
}

// New creates a parent tree allowing to fetch agent's parent and children.
// It ensures that an agent can have at most one parent.
// It ensures that the root node name is not referenced again in the agent tree
func New(root agent.Agent) (*Tree, error) {
	parent := make(map[string]agent.Agent)
	children := make(map[string][]agent.Agent)
	rootName := root.Name()
	byName := map[string]agent.Agent{rootName: root}
	pointerMap := map[agent.Agent]string{root: "is root agent"}

	var f func(cur agent.Agent) error
	f = func(cur agent.Agent) error {
		for _, subAgent := range cur.SubAgents() {
			if p, ok := pointerMap[subAgent]; ok {
				return fmt.Errorf("%q agent cannot have >1 parents, found: %q, %q", subAgent.Name(), p, cur.Name())
			}
			if _, ok := parent[subAgent.Name()]; ok || subAgent.Name() == rootName {
				return fmt.Errorf("agent names must be unique in the agent tree, found duplicate: %q", subAgent.Name())
			}
			parent[subAgent.Name()] = cur
			children[cur.Name()] = append(children[cur.Name()], subAgent)
			byName[subAgent.Name()] = subAgent
			pointerMap[subAgent] = cur.Name()

			if err := f(subAgent); err != nil {
				return err
			}
		}
		return nil
	}

	if err := f(root); err != nil {
		return nil, err
	}
	return &Tree{root: root, parent: parent, children: children, byName: byName}, nil
}

// FindByName returns the agent with the given name, or nil if not found.
func (t *Tree) FindByName(name string) agent.Agent {
	if t == nil {
		return nil
	}
	return t.byName[name]
}

// RootAgent returns the root of the agent tree.
func (t *Tree) RootAgent(cur agent.Agent) agent.Agent {
	if cur == nil || t == nil {
		return nil
	}
	for {
		p := t.parent[cur.Name()]
		if p == nil {
			return cur
		}
		cur = p
	}
}

// ParentOf returns the immediate parent of the named agent.
func (t *Tree) ParentOf(name string) agent.Agent {
	if t == nil {
		return nil
	}
	return t.parent[name]
}

// ChildrenOf returns the immediate children of the named agent.
func (t *Tree) ChildrenOf(name string) []agent.Agent {
	if t == nil {
		return nil
	}
	return t.children[name]
}

// SiblingsOf returns all siblings of the named agent (excluding itself).
func (t *Tree) SiblingsOf(name string) []agent.Agent {
	if t == nil {
		return nil
	}
	parent := t.parent[name]
	if parent == nil {
		return nil
	}
	var res []agent.Agent
	for _, child := range t.children[parent.Name()] {
		if child.Name() != name {
			res = append(res, child)
		}
	}
	return res
}

// IsDescendant reports whether childName is a descendant of ancestorName.
func (t *Tree) IsDescendant(childName, ancestorName string) bool {
	if t == nil || childName == "" || ancestorName == "" {
		return false
	}
	for cur := t.parent[childName]; cur != nil; cur = t.parent[cur.Name()] {
		if cur.Name() == ancestorName {
			return true
		}
	}
	return false
}

