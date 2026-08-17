package fgatrace

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/charmbracelet/x/ansi"
	"github.com/openfga/language/pkg/go/graph"
)

var (
	ErrNotFound   = errors.New("vertex not found")
	ErrNoPath     = errors.New("vertex has no path to target")
	ErrDegreeZero = errors.New("vertex has no edges")
)

func wtoc(weight int) ansi.BasicColor {
	switch {
	case weight == 1:
		return lipgloss.BrightGreen
	case weight == 2:
		return lipgloss.BrightYellow
	case weight > 2 && weight < graph.Infinite:
		return lipgloss.BrightMagenta
	default:
		return lipgloss.BrightRed
	}
}

type decorator struct {
	graph      *graph.WeightedAuthorizationModelGraph
	visualizer *Visualizer
	edges      []*graph.WeightedAuthorizationModelEdge
}

func (d *decorator) enumerate(children tree.Children, i int) string {
	var edge *graph.WeightedAuthorizationModelEdge
	if len(d.edges) > i {
		edge = d.edges[i]
	}

	weight, _ := d.graph.GetEdgeWeight(edge, d.visualizer.Target)

	var strWeight string
	if weight == graph.Infinite {
		strWeight = "\u221E"
	} else {
		strWeight = strconv.Itoa(weight)
	}

	separator := "──"

	if d.visualizer.Weight {
		separator += "[" + strWeight + "]"
	}

	if d.visualizer.Kind {
		var kind string

		switch edge.GetEdgeType() {
		case graph.DirectEdge:
			kind = "D"
		case graph.ComputedEdge:
			kind = "C"
		case graph.RewriteEdge:
			kind = "R"
		case graph.TTUEdge:
			kind = "T"
		case graph.DirectLogicalEdge, graph.TTULogicalEdge:
			kind = "L"
		default:
			kind = "?"
		}
		separator += "[" + kind + "]"
	}

	separator += "──"

	if i == children.Length()-1 {
		return "└" + separator
	}
	return "├" + separator
}

func (d *decorator) indent(children tree.Children, i int) string {
	padding := 4

	if d.visualizer.Weight {
		var edge *graph.WeightedAuthorizationModelEdge
		if len(d.edges) > i {
			edge = d.edges[i]
		}

		weight, _ := d.graph.GetEdgeWeight(edge, d.visualizer.Target)

		padding += 3

		if weight > 9 && weight < math.MaxInt32 {
			padding += 1
		}
	}

	if d.visualizer.Kind {
		padding += 3
	}

	if i == children.Length()-1 {
		return strings.Repeat(" ", padding+1)
	}
	return "│" + strings.Repeat(" ", padding)
}

func (d *decorator) style(children tree.Children, i int) lipgloss.Style {
	style := lipgloss.NewStyle()

	if !d.visualizer.Color {
		return style
	}

	var edge *graph.WeightedAuthorizationModelEdge
	if len(d.edges) > i {
		edge = d.edges[i]
	} else {
		return style
	}

	weight, _ := d.graph.GetEdgeWeight(edge, d.visualizer.Target)
	color := wtoc(weight)
	style = style.Foreground(color)
	return style
}

type Visualizer struct {
	Source string
	Target string
	Weight bool
	Color  bool
	Detail bool
	Kind   bool
}

func (v *Visualizer) label(node *graph.WeightedAuthorizationModelNode) string {
	var label string
	if v.Detail {
		label = node.GetUniqueLabel()
	} else {
		label = node.GetLabel()
	}

	if node.GetRecursiveRelation() != "" {
		label += " \u21BB"
	}

	if node.IsPartOfTupleCycle() {
		label += " \u267A"
	}
	return label
}

func (v *Visualizer) decorate(
	g *graph.WeightedAuthorizationModelGraph,
	t *tree.Tree,
	edges []*graph.WeightedAuthorizationModelEdge,
) {
	d := decorator{
		graph:      g,
		visualizer: v,
		edges:      edges,
	}
	t.Enumerator(d.enumerate)
	t.Indenter(d.indent)
	t.ItemStyleFunc(d.style)
}

func (v *Visualizer) Traverse(g *graph.WeightedAuthorizationModelGraph) error {
	node, ok := g.GetNodeByID(v.Source)
	if !ok {
		return ErrNotFound
	}

	sourceWeight, ok := g.GetNodeWeight(node, v.Target)
	if !ok {
		return ErrNoPath
	}

	edges, ok := g.GetEdgesFromNode(node)
	if !ok {
		return ErrDegreeZero
	}

	stack := make([]*item, 0, len(edges))

	for _, edge := range slices.Backward(edges) {
		stack = append(stack, &item{node, edge, 0})
	}

	root := tree.Root(v.label(node))

	parents := []*frame{
		{
			node:   node,
			branch: root,
		},
	}

	var visited []*item

	for len(stack) > 0 {
		ndx := len(stack) - 1
		i := stack[ndx]
		stack = stack[:ndx]

		edge := i.edge
		depth := i.depth
		node := i.parent

		j := len(visited)
		for j > 0 && visited[j-1].depth > depth {
			j--
		}

		visited = visited[:j]

		_, ok := g.GetEdgeWeight(edge, v.Target)
		if !ok {
			continue
		}

		var found bool
		for _, i := range visited {
			if i.edge == edge {
				found = true
				break
			}
		}

		if found {
			continue
		}

		visited = append(visited, i)
		depth++

		to := edge.GetTo()

		var parent *frame

		for len(parents) > 0 {
			ndx := len(parents) - 1
			parent = parents[ndx]

			if parent.node == node {
				break
			}

			v.decorate(g, parent.branch, parent.edges)
			parents = parents[:ndx]
		}

		var l string

		edgeType := edge.GetEdgeType()

		if edgeType == graph.TTUEdge {
			l += edge.GetTuplesetRelation() + " \u21A6 "
		}

		l += v.label(to)

		var ancestor *graph.WeightedAuthorizationModelNode

		if v.Detail || (edgeType != graph.DirectLogicalEdge && edgeType != graph.TTULogicalEdge) {
			child := tree.New().Root(l)
			parent.edges = append(parent.edges, edge)
			parent.branch.Child(child)
			parents = append(parents, &frame{to, child, nil})
			ancestor = to
		} else {
			ancestor = parent.node
		}

		edges, ok := g.GetEdgesFromNode(to)
		if !ok {
			continue
		}

		for _, edge := range slices.Backward(edges) {
			stack = append(stack, &item{ancestor, edge, depth})
		}
	}

	for len(parents) > 0 {
		ndx := len(parents) - 1
		v.decorate(g, parents[ndx].branch, parents[ndx].edges)
		parents = parents[:ndx]
	}

	if v.Color {
		color := wtoc(sourceWeight)
		root.RootStyle(lipgloss.NewStyle().Foreground(color))
	}

	_, err := lipgloss.Println(root)
	if err != nil {
		return err
	}
	return nil
}

type item struct {
	parent *graph.WeightedAuthorizationModelNode
	edge   *graph.WeightedAuthorizationModelEdge
	depth  int
}

type frame struct {
	node   *graph.WeightedAuthorizationModelNode
	branch *tree.Tree
	edges  []*graph.WeightedAuthorizationModelEdge
}
