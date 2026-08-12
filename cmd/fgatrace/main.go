package main

import (
	"fgatrace"
	"flag"
	"io"
	"log"
	"os"

	"github.com/openfga/language/pkg/go/graph"
	"github.com/openfga/language/pkg/go/transformer"
)

func main() {
	var visualizer fgatrace.Visualizer

	options := flag.NewFlagSet("options", flag.ExitOnError)
	options.StringVar(&visualizer.Source, "source", "", "relation that will serve as the traversal entry point.")
	options.StringVar(&visualizer.Target, "target", "", "specific type that will serve as the traversal terminal point.")
	options.BoolVar(&visualizer.Weight, "weight", false, "show edge weights")
	options.BoolVar(&visualizer.Color, "color", false, "show weight coloration")
	options.BoolVar(&visualizer.Detail, "detail", false, "show detailed node labels")
	options.BoolVar(&visualizer.Kind, "type", false, "show edge types")
	allPtr := options.Bool("all", false, "enable all options")

	err := options.Parse(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	file := options.Arg(0)

	if file == "" {
		log.Fatal("no model file provided")
	}

	if visualizer.Source == "" {
		log.Fatal("no source provided")
	}

	if visualizer.Target == "" {
		log.Fatal("no target provided")
	}

	if *allPtr {
		visualizer.Weight = !visualizer.Weight
		visualizer.Color = !visualizer.Color
		visualizer.Detail = !visualizer.Detail
		visualizer.Kind = !visualizer.Kind
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
		log.Fatal(err)
	}

	err = visualizer.Traverse(g)
	if err != nil {
		log.Fatal(err)
	}
}
