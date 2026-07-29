package main

import (
	"flag"
	"io"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/openfga/language/pkg/go/graph"
	"github.com/openfga/language/pkg/go/transformer"
)

type frame struct {
	node    *graph.WeightedAuthorizationModelNode
	branch  *tree.Tree
	weights []int
}

func main() {
	stdinPtr := flag.Bool("stdin", false, "accept model dsl from stdin")
	weightPtr := flag.Bool("weight", false, "show edge weights")
	colorPtr := flag.Bool("color", false, "show weight coloration")
	detailPtr := flag.Bool("detail", false, "show detailed node labels")
	sourcePtr := flag.String("source", "", "origin specific type and relation node label")
	targetPtr := flag.String("target", "", "destination specific type node label")

	flag.Parse()

	if *sourcePtr == "" {
		log.Fatal("no source provided")
	}

	if *targetPtr == "" {
		log.Fatal("no target provided")
	}

	var reader io.Reader

	if *stdinPtr {
		reader = io.LimitReader(os.Stdin, 100*1024)
	}

	if reader == nil {
		log.Fatal("no input method indicated")
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		log.Fatalf("unable to read input: %s", err)
	}

	model := transformer.MustTransformDSLToProto(string(data))

	builder := graph.NewWeightedAuthorizationModelGraphBuilder()

	g, err := builder.Build(model)
	if err != nil {
		panic(err)
	}

	node, ok := g.GetNodeByID(*sourcePtr)
	if !ok {
		log.Fatal("node not found")
	}

	edges, ok := g.GetEdgesFromNode(node)
	if !ok {
		log.Fatal("edges not found")
	}

	stack := make([]*graph.WeightedAuthorizationModelEdge, len(edges))
	copy(stack, edges)
	slices.Reverse(stack)

	label := func(node *graph.WeightedAuthorizationModelNode) string {
		var label string
		if *detailPtr {
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

	root := tree.Root(label(node))

	parents := []*frame{
		{
			node:   node,
			branch: root,
		},
	}

	var visited []*graph.WeightedAuthorizationModelEdge

	applyStyles := func(t *tree.Tree, weights []int) {
		t.Enumerator(func(children tree.Children, i int) string {
			var weight int
			if len(weights) > i {
				weight = weights[i]
			}

			var strWeight string
			if weight == graph.Infinite {
				strWeight = "\u221E"
			} else {
				strWeight = strconv.Itoa(weight)
			}

			var separator string

			if *weightPtr {
				separator = "─[" + strWeight + "]─" + strings.Repeat("─", 2-utf8.RuneCountInString(strWeight))
			} else {
				separator = "────"
			}

			if i == children.Length()-1 {
				return "└" + separator
			}
			return "├" + separator
		})

		t.Indenter(func(children tree.Children, i int) string {
			if *weightPtr {
				if i == children.Length()-1 {
					return "       "
				}
				return "│      "
			}

			if i == children.Length()-1 {
				return "     "
			}
			return "│    "
		})

		t.ItemStyleFunc(func(children tree.Children, i int) lipgloss.Style {
			style := lipgloss.NewStyle()

			if !*colorPtr {
				return style
			}

			var weight int
			if len(weights) > i {
				weight = weights[i]
			}

			switch {
			case weight == 1:
				style = style.Foreground(lipgloss.BrightGreen)
			case weight == 2:
				style = style.Foreground(lipgloss.BrightYellow)
			case weight > 2 && weight < graph.Infinite:
				style = style.Foreground(lipgloss.BrightMagenta)
			default:
				style = style.Foreground(lipgloss.BrightRed)
			}
			return style
		})
	}

	for len(stack) > 0 {
		ndx := len(stack) - 1
		edge := stack[ndx]
		stack = stack[:ndx]

		weight, ok := edge.GetWeight(*targetPtr)
		if !ok {
			continue
		}

		if slices.Contains(visited, edge) {
			continue
		}

		visited = append(visited, edge)

		from := edge.GetFrom()
		to := edge.GetTo()

		for len(parents) > 0 && parents[len(parents)-1].node != from {
			ndx := len(parents) - 1
			applyStyles(parents[ndx].branch, parents[ndx].weights)
			parents = parents[:ndx]
			visited = visited[:len(visited)-1]
		}

		if edge.GetEdgeType() == graph.TTUEdge {
			child := tree.New().Root(edge.GetTuplesetRelation())
			parent := parents[len(parents)-1]
			parent.weights = append(parent.weights, weight)
			parent.branch.Child(child)
			parents = append(parents, &frame{to, child, nil})
		}

		child := tree.New().Root(label(to))
		parent := parents[len(parents)-1]
		parent.weights = append(parent.weights, weight)
		parent.branch.Child(child)
		parents = append(parents, &frame{to, child, nil})

		edges, ok := g.GetEdgesFromNode(to)
		if !ok {
			continue
		}

		next := make([]*graph.WeightedAuthorizationModelEdge, len(edges))
		copy(next, edges)
		slices.Reverse(next)

		stack = append(stack, next...)
	}

	for len(parents) > 0 {
		ndx := len(parents) - 1
		applyStyles(parents[ndx].branch, parents[ndx].weights)
		parents = parents[:ndx]
	}

	if *colorPtr {
		root.RootStyle(lipgloss.NewStyle().Foreground(lipgloss.BrightBlue).Bold(true))
	}

	_, err = lipgloss.Println(root)
	if err != nil {
		log.Fatal(err)
	}
}
