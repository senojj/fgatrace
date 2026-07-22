package main

import (
	"flag"
	"io"
	"log"
	"os"
	"strconv"

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
	var strWeight string
	if weight == graph.Infinite {
		strWeight = "INF"
	} else {
		strWeight = strconv.Itoa(weight)
	}
	return name + " (W=" + strWeight + ")"
}

func main() {
	stdinPtr := flag.Bool("stdin", false, "accept model dsl from stdin")
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

	_, err = lipgloss.Println(root)
	if err != nil {
		log.Fatal(err)
	}
}
