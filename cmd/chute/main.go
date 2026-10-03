package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jenksed/chute-go/internal/chute"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "process":
		runProcess(os.Args[2:])
	case "volume":
		runVolume(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func runProcess(args []string) {
	flags := flag.NewFlagSet("process", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	output := flags.String("output", "", "output directory")
	flags.StringVar(output, "o", "", "output directory")

	if err := flags.Parse(args); err != nil {
		os.Exit(1)
	}
	if flags.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	root := flags.Arg(0)
	if *output == "" {
		*output = filepath.Join(mustWorkingDir(), "chute-output")
	}

	bundle, err := chute.Load(root)
	if err != nil {
		fail(err)
	}
	if err := chute.WriteBundle(bundle, *output); err != nil {
		fail(err)
	}

	absolute, _ := filepath.Abs(*output)
	fmt.Printf("Wrote processed bundle to %s\n", absolute)
}

func runVolume(args []string) {
	flags := flag.NewFlagSet("volume", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	output := flags.String("output", "", "output directory")
	flags.StringVar(output, "o", "", "output directory")

	if err := flags.Parse(args); err != nil {
		os.Exit(1)
	}
	if flags.NArg() != 2 {
		usage()
		os.Exit(1)
	}

	root := flags.Arg(0)
	volumeName := flags.Arg(1)
	if *output == "" {
		*output = filepath.Join(mustWorkingDir(), "chute-case-"+chute.SafeName(volumeName))
	}

	bundle, err := chute.Load(root)
	if err != nil {
		fail(err)
	}
	projection, err := chute.ProjectVolume(bundle.Index, volumeName)
	if err != nil {
		fail(err)
	}
	logs := chute.ExtractLogs(bundle.Root, bundle.Inventory, projection.Identifiers)
	if err := chute.WriteProjection(projection, logs, *output); err != nil {
		fail(err)
	}

	absolute, _ := filepath.Abs(*output)
	fmt.Printf("Wrote volume case to %s\n", absolute)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "chute: %v\n", err)
	os.Exit(2)
}

func mustWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	return dir
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  chute process [--output DIR] BUNDLE_DIR
  chute volume [--output DIR] BUNDLE_DIR VOLUME_NAME

BUNDLE_DIR must already be extracted.`)
}
