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
	case "inspect":
		runInspect(os.Args[2:])
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
	contextLines := flags.Int("context", chute.DefaultLogContext, "log lines before and after a match")

	if err := flags.Parse(args); err != nil {
		os.Exit(1)
	}
	if flags.NArg() != 1 || *contextLines < 0 {
		usage()
		os.Exit(1)
	}

	input := flags.Arg(0)
	if *output == "" {
		*output = filepath.Join(mustWorkingDir(), "chute-output")
	}

	bundle, cleanup, err := chute.LoadInput(input)
	if err != nil {
		fail(err)
	}
	defer cleanup()

	if err := chute.WriteBundle(bundle, *output, *contextLines); err != nil {
		fail(err)
	}

	absolute, _ := filepath.Abs(*output)
	fmt.Printf("Wrote processed bundle to %s\n", absolute)
}

func runInspect(args []string) {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	if err := flags.Parse(args); err != nil {
		os.Exit(1)
	}
	if flags.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	bundle, cleanup, err := chute.LoadInput(flags.Arg(0))
	if err != nil {
		fail(err)
	}
	defer cleanup()

	if err := chute.WriteInspection(os.Stdout, chute.InspectVolumes(bundle.Index)); err != nil {
		fail(err)
	}
}

func runVolume(args []string) {
	flags := flag.NewFlagSet("volume", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	output := flags.String("output", "", "output directory")
	flags.StringVar(output, "o", "", "output directory")
	pvc := flags.String("pvc", "", "select volume by PVC name or namespace/name")
	pod := flags.String("pod", "", "select volume by Pod name or namespace/name")
	contextLines := flags.Int("context", chute.DefaultLogContext, "log lines before and after a match")

	if err := flags.Parse(args); err != nil {
		os.Exit(1)
	}
	if *contextLines < 0 || (*pvc != "" && *pod != "") {
		usage()
		os.Exit(1)
	}

	expectedArgs := 2
	if *pvc != "" || *pod != "" {
		expectedArgs = 1
	}
	if flags.NArg() != expectedArgs {
		usage()
		os.Exit(1)
	}

	input := flags.Arg(0)
	bundle, cleanup, err := chute.LoadInput(input)
	if err != nil {
		fail(err)
	}
	defer cleanup()

	volumeName := ""
	switch {
	case *pvc != "":
		volumeName, err = chute.ResolveVolumeByPVC(bundle.Index, *pvc)
	case *pod != "":
		volumeName, err = chute.ResolveVolumeByPod(bundle.Index, *pod)
	default:
		volumeName = flags.Arg(1)
	}
	if err != nil {
		fail(err)
	}

	if *output == "" {
		*output = filepath.Join(mustWorkingDir(), "chute-case-"+chute.SafeName(volumeName))
	}

	projection, err := chute.ProjectVolume(bundle.Index, volumeName)
	if err != nil {
		fail(err)
	}
	logs := chute.ExtractLogs(bundle.Root, bundle.Inventory, projection.Identifiers, *contextLines)
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
  chute inspect INPUT
  chute process [--output DIR] [--context N] INPUT
  chute volume [--output DIR] [--context N] INPUT VOLUME_NAME
  chute volume [--output DIR] [--context N] --pvc [NAMESPACE/]PVC INPUT
  chute volume [--output DIR] [--context N] --pod [NAMESPACE/]POD INPUT

INPUT may be an extracted directory, .zip, .tar.gz, or .tgz archive.`)
}
