// Command host-check runs the validation host contract before any performance
// number is recorded: it prints what this host is and exits non-zero when the
// host may not produce reference performance evidence.
//
//	just bench-host-check
//	go run ./bench/hostcheck/cmd/host-check -json    # the recordable form
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/elcuervo/emb/bench/hostcheck"
)

func main() {
	asJSON := flag.Bool("json", false, "print the report as JSON")
	quiet := flag.Bool("quiet", false, "print only the verdict and the failed property")
	sample := flag.Duration("sample", time.Second, "window the steal and throttle deltas are sampled over")
	platform := flag.String("platform", hostcheck.DefaultPlatform, "deployment target as os/arch")
	flag.Parse()

	th := hostcheck.DefaultThresholds()
	th.Platform = *platform
	th.SampleWindow = *sample
	report := hostcheck.Probe(th)

	switch {
	case *asJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "host-check: %v\n", err)
			os.Exit(2)
		}
	case *quiet:
		line := report.Verdict
		if report.FailedProperty != "" {
			line += ": " + report.FailedProperty
		}
		fmt.Println(line)
	default:
		fmt.Print(report.String())
	}
	if !report.Qualified() {
		os.Exit(1)
	}
}
