package main

import (
	"flag"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/openfga/language/pkg/go/graph"
	"github.com/openfga/language/pkg/go/transformer"
)

type frame struct {
	label  string
	branch *tree.Tree
}

func label(name string, weight int) string {
	if weight == 0 {
		return name
	}
	return name + "\x00" + strconv.Itoa(weight)
}

func main() {
	stdinPtr := flag.Bool("stdin", false, "accept model dsl from stdin")
	weightedPtr := flag.Bool("weighted", false, "show edge weights")
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

	visited := make(map[*graph.WeightedAuthorizationModelEdge]struct{})

	root := tree.Root(*sourcePtr)

	stack := []frame{
		{
			label:  *sourcePtr,
			branch: root,
		},
	}

	for len(edges) > 0 {
		ndx := len(edges) - 1
		edge := edges[ndx]
		edges = edges[:ndx]

		weight, ok := edge.GetWeight(*targetPtr)
		if !ok {
			continue
		}

		if _, ok := visited[edge]; ok {
			visited = make(map[*graph.WeightedAuthorizationModelEdge]struct{})
			continue
		}

		visited[edge] = struct{}{}

		from := edge.GetFrom().GetUniqueLabel()
		to := edge.GetTo().GetUniqueLabel()

		for len(stack) > 0 && stack[len(stack)-1].label != from {
			stack = stack[:len(stack)-1]
		}

		child := tree.New().Root(label(to, weight))
		stack[len(stack)-1].branch.Child(child)
		stack = append(stack, frame{to, child})

		next, ok := g.GetEdgesFromNode(edge.GetTo())
		if !ok {
			continue
		}
		edges = append(edges, next...)
	}

	root.ItemStyleFunc(func(children tree.Children, i int) lipgloss.Style {
		style := lipgloss.NewStyle()

		child := children.At(i)

		n, w, found := strings.Cut(child.Value(), "\x00")

		if !*weightedPtr {
			child.SetValue(n)
		} else {
			child.SetValue(n + " (w=" + w + ")")
		}
		if !found {
			return style
		}

		weight, err := strconv.Atoi(w)
		if err != nil {
			return style
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

	_, err = lipgloss.Println(root)
	if err != nil {
		log.Fatal(err)
	}
}
