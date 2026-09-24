// Command bench replays a hand-labelled corpus through `ohmylaya ask` and
// reports accuracy, estimated model tokens with and without the tools, and
// engine time. Run it from the repository root:
//
//	go run ./bench -bin bin/ohmylaya
//
// It needs an installed engine; see docs/benchmarks.md for the method.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/QuBiit0/ohmylaya/internal/tools"
)

func main() {
	bin := flag.String("bin", "ohmylaya", "ohmylaya executable")
	corpus := flag.String("corpus", "testdata/bench", "directory of labelled suites")
	timeout := flag.Duration("timeout", defaultTimeout, "deadline for each case")
	flag.Parse()
	suites, err := LoadSuites(*corpus)
	if err == nil && len(suites) == 0 {
		err = fmt.Errorf("no suites in %s", *corpus)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
	reader := tools.NewReader(nil)
	var results []Result
	failed := false
	for _, s := range suites {
		s.Timeout = *timeout
		results = append(results, Run(context.Background(), s, askRunner(*bin), reader))
		failed = failed || results[len(results)-1].Failed()
	}
	Report(os.Stdout, results)
	if failed {
		os.Exit(1)
	}
}

// askRunner calls `<bin> ask <tool>` with the input on stdin.
func askRunner(bin string) Runner {
	return func(ctx context.Context, tool string, input []byte) ([]byte, error) {
		cmd := exec.CommandContext(ctx, bin, "ask", tool)
		cmd.Stdin = bytes.NewReader(input)
		out, err := cmd.Output()
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%w: %s", err, bytes.TrimSpace(ee.Stderr))
		}
		return out, err
	}
}
