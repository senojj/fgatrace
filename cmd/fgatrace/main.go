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

	root := tree.Root(*sourcePtr)

	stack := []*frame{
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

	for len(edges) > 0 {
		ndx := len(edges) - 1
		edge := edges[ndx]
		edges = edges[:ndx]

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

		for len(stack) > 0 && stack[len(stack)-1].node != from {
			ndx := len(stack) - 1
			applyStyles(stack[ndx].branch, stack[ndx].weights)
			stack = stack[:ndx]
			visited = visited[:len(visited)-1]
		}

		child := tree.New().Root(to.GetUniqueLabel())
		parent := stack[len(stack)-1]
		parent.weights = append(parent.weights, weight)
		parent.branch.Child(child)
		stack = append(stack, &frame{to, child, nil})

		next, ok := g.GetEdgesFromNode(to)
		if !ok {
			continue
		}
		edges = append(edges, next...)
	}

	for len(stack) > 0 {
		ndx := len(stack) - 1
		applyStyles(stack[ndx].branch, stack[ndx].weights)
		stack = stack[:ndx]
	}

	if *colorPtr {
		root.RootStyle(lipgloss.NewStyle().Foreground(lipgloss.BrightBlue).Bold(true))
	}

	_, err = lipgloss.Println(root)
	if err != nil {
		log.Fatal(err)
	}
}
