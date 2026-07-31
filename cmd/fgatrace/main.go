package main

import (
	"flag"
	"io"
	"log"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/charmbracelet/x/ansi"
	"github.com/openfga/language/pkg/go/graph"
	"github.com/openfga/language/pkg/go/transformer"
)

type item struct {
	edge  *graph.WeightedAuthorizationModelEdge
	depth int
}

type frame struct {
	node   *graph.WeightedAuthorizationModelNode
	branch *tree.Tree
	edges  []*graph.WeightedAuthorizationModelEdge
}

func main() {
	options := flag.NewFlagSet("options", flag.ExitOnError)
	sourcePtr := options.String("source", "", "relation that will serve as the traversal entry point.")
	targetPtr := options.String("target", "", "specific type that will serve as the traversal terminal point.")
	weightPtr := options.Bool("weight", false, "show edge weights")
	colorPtr := options.Bool("color", false, "show weight coloration")
	detailPtr := options.Bool("detail", false, "show detailed node labels")
	typePtr := options.Bool("type", false, "show edge types")
	allPtr := options.Bool("all", false, "enable all options")

	err := options.Parse(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	file := options.Arg(0)

	if file == "" {
		log.Fatal("no model file provided")
	}

	source := *sourcePtr
	target := *targetPtr

	if source == "" {
		log.Fatal("no source provided")
	}

	if target == "" {
		log.Fatal("no target provided")
	}

	if *allPtr {
		*weightPtr = !*weightPtr
		*colorPtr = !*colorPtr
		*detailPtr = !*detailPtr
		*typePtr = !*typePtr
	}

	var reader io.Reader

	reader, err = os.Open(file)
	if err != nil {
		log.Fatal(err)
	}

	reader = io.LimitReader(reader, 100*1024)

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

	node, ok := g.GetNodeByID(source)
	if !ok {
		log.Fatal("node not found")
	}

	sourceWeight, ok := node.GetWeight(target)
	if !ok {
		log.Fatal("source has no path to target")
	}

	edges, ok := g.GetEdgesFromNode(node)
	if !ok {
		log.Fatal("edges not found")
	}

	stack := make([]*item, 0, len(edges))

	for _, edge := range slices.Backward(edges) {
		stack = append(stack, &item{edge, 0})
	}

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

	var visited []*item

	wtoc := func(weight int) ansi.BasicColor {
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

	applyStyles := func(t *tree.Tree, edges []*graph.WeightedAuthorizationModelEdge) {
		t.Enumerator(func(children tree.Children, i int) string {
			var edge *graph.WeightedAuthorizationModelEdge
			if len(edges) > i {
				edge = edges[i]
			}

			weight, _ := edge.GetWeight(target)

			var strWeight string
			if weight == graph.Infinite {
				strWeight = "\u221E"
			} else {
				strWeight = strconv.Itoa(weight)
			}

			separator := "──"

			if *weightPtr {
				separator += "[" + strWeight + "]"
			}

			if *typePtr {
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
		})

		t.Indenter(func(children tree.Children, i int) string {
			padding := 4

			if *weightPtr {
				var edge *graph.WeightedAuthorizationModelEdge
				if len(edges) > i {
					edge = edges[i]
				}

				weight, _ := edge.GetWeight(target)

				padding += 3

				if weight > 9 && weight < math.MaxInt32 {
					padding += 1
				}
			}

			if *typePtr {
				padding += 3
			}

			if i == children.Length()-1 {
				return strings.Repeat(" ", padding+1)
			}
			return "│" + strings.Repeat(" ", padding)
		})

		t.ItemStyleFunc(func(children tree.Children, i int) lipgloss.Style {
			style := lipgloss.NewStyle()

			if !*colorPtr {
				return style
			}

			var edge *graph.WeightedAuthorizationModelEdge
			if len(edges) > i {
				edge = edges[i]
			} else {
				return style
			}

			weight, _ := edge.GetWeight(target)
			color := wtoc(weight)
			style = style.Foreground(color)
			return style
		})
	}

	for len(stack) > 0 {
		ndx := len(stack) - 1
		i := stack[ndx]
		stack = stack[:ndx]

		edge := i.edge
		depth := i.depth

		j := len(visited)
		for j > 0 && visited[j-1].depth > depth {
			j--
		}

		visited = visited[:j]

		_, ok := edge.GetWeight(target)
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

		from := edge.GetFrom()
		to := edge.GetTo()

		var parent *frame

		for len(parents) > 0 {
			ndx := len(parents) - 1
			parent = parents[ndx]

			if parent.node == from {
				break
			}

			applyStyles(parent.branch, parent.edges)
			parents = parents[:ndx]
		}

		var l string

		if edge.GetEdgeType() == graph.TTUEdge {
			l += edge.GetTuplesetRelation() + " \u21A6 "
		}

		l += label(to)

		child := tree.New().Root(l)
		parent.edges = append(parent.edges, edge)
		parent.branch.Child(child)
		parents = append(parents, &frame{to, child, nil})

		edges, ok := g.GetEdgesFromNode(to)
		if !ok {
			continue
		}

		for _, edge := range slices.Backward(edges) {
			stack = append(stack, &item{edge, depth})
		}
	}

	for len(parents) > 0 {
		ndx := len(parents) - 1
		applyStyles(parents[ndx].branch, parents[ndx].edges)
		parents = parents[:ndx]
	}

	if *colorPtr {
		color := wtoc(sourceWeight)
		root.RootStyle(lipgloss.NewStyle().Foreground(color))
	}

	_, err = lipgloss.Println(root)
	if err != nil {
		log.Fatal(err)
	}
}
