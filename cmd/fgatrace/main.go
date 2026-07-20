package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/openfga/language/pkg/go/graph"
	"github.com/openfga/language/pkg/go/transformer"
)

const (
	inflen  = 3
	padding = inflen + 6
)

var (
	visited   map[*graph.WeightedAuthorizationModelEdge]struct{}
	stack     []string
	separator = []byte(" ")
)

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

	println("+ \x1b[1m" + *sourcePtr + "\x1b[0m")

	for len(edges) > 0 {
		ndx := len(edges) - 1
		edge := edges[ndx]
		edges = edges[:ndx]

		var weight int
		var ok bool

		if weight, ok = edge.GetWeight(*targetPtr); !ok {
			continue
		}

		if _, ok := visited[edge]; ok {
			visited = nil
			continue
		}

		if weight == graph.Infinite {
			if visited == nil {
				visited = make(map[*graph.WeightedAuthorizationModelEdge]struct{})
			}
			visited[edge] = struct{}{}
		}

		from := edge.GetFrom().GetUniqueLabel()
		to := edge.GetTo().GetUniqueLabel()

		for len(stack) > 0 && from != stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
		}

		prefix := string(bytes.Repeat(append([]byte{'|'}, bytes.Repeat(separator, padding)...), len(stack)))

		var strWeight string
		if weight == graph.Infinite {
			strWeight = "INF"
		} else {
			strWeight = strconv.Itoa(weight)
		}

		println(prefix + "|")
		println(fmt.Sprintf("%s|--\x1b[1mW=%s\x1b[0m%s--+ \x1b[1m%s\x1b[0m", prefix, strWeight, strings.Repeat("-", inflen-len(strWeight)), to))

		next, ok := g.GetEdgesFromNode(edge.GetTo())
		if !ok {
			continue
		}

		edges = append(edges, next...)
		stack = append(stack, to)
	}
	println("")
}
