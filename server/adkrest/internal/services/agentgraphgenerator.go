package services

import (
	"context"
	"fmt"
	"slices"

	"github.com/awalterschulze/gographviz"

	"github.com/nickqiaoo/tadk/agent"
	llmagentinternal "github.com/nickqiaoo/tadk/internal/llminternal"
	"github.com/nickqiaoo/tadk/tool"
)

const (
	DarkGreen  = "\"#0F5223\""
	LightGreen = "\"#69CB87\""
	LightGray  = "\"#cccccc\""
	White      = "\"#ffffff\""
	Background = "\"#333537\""
)

var supportedClusterAgents = []agent.Type{
	agent.TypeLoopAgent,
	agent.TypeSequentialAgent,
	agent.TypeParallelAgent,
}

type namedInstance interface {
	Name() string
}

func nodeName(instance any) string {
	switch i := instance.(type) {
	case agent.Agent:
		return i.Name()
	case tool.Tool:
		return i.Name()
	default:
		return "Unknown instance type"
	}
}

func nodeCaption(instance any) string {
	caption := ""
	switch i := instance.(type) {
	case agent.Agent:
		caption = "🤖 " + i.Name()
		agentType := agent.TypeOf(i)
		if slices.Contains(supportedClusterAgents, agentType) {
			caption = i.Name() + " (" + string(agentType) + ")"
		}
	case tool.Tool:
		caption = "🔧 " + i.Name()
	default:
		caption = "Unsupported agent or tool type"
	}
	return "\"" + caption + "\""
}

func nodeShape(instance any) string {
	switch instance.(type) {
	case agent.Agent:
		return "ellipse"
	case tool.Tool:
		return "box"
	default:
		return "cylinder"
	}
}

func shouldBuildAgentCluster(instance any) bool {
	switch i := instance.(type) {
	case agent.Agent:
		return slices.Contains(supportedClusterAgents, agent.TypeOf(i))
	default:
		return false
	}
}

func highlighted(nodeName string, higlightedPairs [][]string) bool {
	if len(higlightedPairs) == 0 {
		return false
	}
	for _, pair := range higlightedPairs {
		if slices.Contains(pair, nodeName) {
			return true
		}
	}
	return false
}

type edgeHighlightDirection uint8

const (
	edgeHighlightNone edgeHighlightDirection = iota
	edgeHighlightForward
	edgeHighlightBackward
)

// Function returns whether the edge should be highlighted.
// The graph could have the pairs highlighted in different directions.
// If edgeHighlightNone is returned, the nodes aren't highlighted.
// edgeHighlightForward means the directed connection is highlighted.
// edgeHighlightBackward means the pair is highlighted in the reverse direction.
func edgeHighlighted(from, to string, higlightedPairs [][]string) edgeHighlightDirection {
	if len(higlightedPairs) == 0 {
		return edgeHighlightNone
	}
	for _, pair := range higlightedPairs {
		if len(pair) == 2 {
			if pair[0] == from && pair[1] == to {
				return edgeHighlightForward
			}
			if pair[0] == to && pair[1] == from {
				return edgeHighlightBackward
			}
		}
	}
	return edgeHighlightNone
}

func drawCluster(parentGraph, cluster *gographviz.Graph, ag agent.Agent, highlightedPairs [][]string, visitedNodes map[string]bool) error {
	for i, subAgent := range ag.SubAgents() {
		err := buildGraph(cluster, parentGraph, subAgent, highlightedPairs, visitedNodes)
		if err != nil {
			return fmt.Errorf("draw cluster: build graph: %w", err)
		}
		switch agent.TypeOf(ag) {
		// Sequential sub-agents should be connected one after another with edges.
		case agent.TypeSequentialAgent:
			if i < len(ag.SubAgents())-1 {
				err = drawEdge(parentGraph, nodeName(subAgent), nodeName(ag.SubAgents()[i+1]), highlightedPairs)
				if err != nil {
					return fmt.Errorf("draw cluster: draw edge: %w", err)
				}
			}
		// Sequential sub-agents should be connected one after another with edges, but the last one should point to the first agent.
		case agent.TypeLoopAgent:
			nextAgentIdx := i + 1
			if nextAgentIdx >= len(ag.SubAgents()) {
				nextAgentIdx = 0
			}
			err = drawEdge(parentGraph, nodeName(subAgent), nodeName(ag.SubAgents()[nextAgentIdx]), highlightedPairs)
			if err != nil {
				return fmt.Errorf("draw cluster: draw edge: %w", err)
			}
		}
		// Parallel sub-agents shouldn't be connected, they will be a part of the sub graph.
	}
	return nil
}

func drawNode(graph, parentGraph *gographviz.Graph, instance any, highlightedPairs [][]string, visitedNodes map[string]bool) error {
	name := nodeName(instance)
	shape := nodeShape(instance)
	caption := nodeCaption(instance)
	highlighted := highlighted(name, highlightedPairs)
	isCluster := shouldBuildAgentCluster(instance)

	visitedNodes[name] = true
	if isCluster {
		agent, ok := instance.(agent.Agent)
		if !ok {
			return nil
		}
		cluster := gographviz.NewGraph()
		err := cluster.SetName("cluster_" + name)
		if err != nil {
			return fmt.Errorf("set cluster name: %w", err)
		}
		err = graph.AddSubGraph(graph.Name, cluster.Name, map[string]string{
			"style":     "rounded",
			"color":     White,
			"label":     caption,
			"fontcolor": LightGray,
		})
		if err != nil {
			return fmt.Errorf("add cluster: %w", err)
		}
		return drawCluster(graph, cluster, agent, highlightedPairs, visitedNodes)
	} else {
		nodeAttributes := map[string]string{
			"label":     caption,
			"shape":     shape,
			"fontcolor": LightGray,
		}

		if highlighted {
			nodeAttributes["color"] = DarkGreen
			nodeAttributes["style"] = "filled"
		} else {
			nodeAttributes["color"] = LightGray
			nodeAttributes["style"] = "rounded"
		}
		return parentGraph.AddNode(graph.Name, name, nodeAttributes)
	}
}

func drawEdge(graph *gographviz.Graph, from, to string, highlightedPairs [][]string) error {
	edgeHighlighted := edgeHighlighted(from, to, highlightedPairs)
	edgeAttributes := map[string]string{}
	if edgeHighlighted != edgeHighlightNone {
		edgeAttributes["color"] = LightGreen
		if edgeHighlighted == edgeHighlightBackward {
			edgeAttributes["arrowhead"] = "normal"
			edgeAttributes["dir"] = "back"
		} else {
			edgeAttributes["arrowhead"] = "normal"
		}
	} else {
		edgeAttributes["color"] = LightGray
		edgeAttributes["arrowhead"] = "none"
	}
	return graph.AddEdge(from, to, true, edgeAttributes)
}

func buildGraph(graph, parentGraph *gographviz.Graph, instance any, highlightedPairs [][]string, visitedNodes map[string]bool) error {
	namedInstance, ok := instance.(namedInstance)
	if !ok {
		return nil
	}
	if visitedNodes[namedInstance.Name()] {
		return nil
	}

	err := drawNode(graph, parentGraph, instance, highlightedPairs, visitedNodes)
	if err != nil {
		return fmt.Errorf("draw node: %w", err)
	}
	agent, ok := instance.(agent.Agent)
	if !ok {
		return nil
	}
	llmAgent, ok := instance.(llmagentinternal.ToolCarrier)
	if ok {
		tools := llmAgent.DeclaredTools()
		for _, tool := range tools {
			err = drawNode(graph, parentGraph, tool, highlightedPairs, visitedNodes)
			if err != nil {
				return fmt.Errorf("draw tool node: %w", err)
			}
			err = drawEdge(graph, nodeName(agent), nodeName(tool), highlightedPairs)
			if err != nil {
				return fmt.Errorf("draw tool edge: %w", err)
			}
		}
	}
	for _, subAgent := range agent.SubAgents() {
		err = buildGraph(graph, parentGraph, subAgent, highlightedPairs, visitedNodes)
		if err != nil {
			return fmt.Errorf("build sub agent graph: %w", err)
		}
	}
	return nil
}

func GetAgentGraph(ctx context.Context, agent agent.Agent, highlightedPairs [][]string) (string, error) {
	graph := gographviz.NewGraph()
	if err := graph.SetName("AgentGraph"); err != nil {
		return "", fmt.Errorf("set graph name: %w", err)
	}
	if err := graph.SetDir(true); err != nil {
		return "", fmt.Errorf("set graph direction: %w", err)
	}
	if err := graph.AddAttr(graph.Name, "rankdir", "LR"); err != nil {
		return "", fmt.Errorf("set graph rank direction: %w", err)
	}
	if err := graph.AddAttr(graph.Name, "bgcolor", Background); err != nil {
		return "", fmt.Errorf("set graph background color: %w", err)
	}
	visitedNodes := map[string]bool{}
	err := buildGraph(graph, graph, agent, highlightedPairs, visitedNodes)
	if err != nil {
		return "", fmt.Errorf("build root graph: %w", err)
	}
	return graph.String(), nil
}
