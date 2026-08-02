// Package status implements the deliberately narrow public API status command.
package status

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/dytonpictures/werk/cmd/werkctl/internal/apiinspect"
	"github.com/dytonpictures/werk/cmd/werkctl/internal/diagnostic"
)

const (
	defaultTimeout = 5 * time.Second
	minimumTimeout = 100 * time.Millisecond
	maximumTimeout = time.Minute
)

type options struct {
	baseURL string
	json    bool
	timeout time.Duration
}

// Command reads only WERK's unauthenticated metadata and health endpoints.
// It does not infer worker, Kafka, migration, host or deployment status.
func Command(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	return commandWithGetter(ctx, arguments, stdout, stderr, nil)
}

func commandWithGetter(
	ctx context.Context,
	arguments []string,
	stdout, stderr io.Writer,
	getter apiinspect.Getter,
) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	commandOptions := options{timeout: defaultTimeout}
	flags.StringVar(&commandOptions.baseURL, "url", "", "explicit WERK API base URL (required)")
	flags.BoolVar(&commandOptions.json, "json", false, "write the stable JSON report")
	flags.DurationVar(&commandOptions.timeout, "timeout", defaultTimeout, "timeout per public API request")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: werkctl status --url URL [--json] [--timeout DURATION]")
	}
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return diagnostic.ExitHealthy
		}
		return diagnostic.ExitFailure
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return diagnostic.ExitFailure
	}
	if commandOptions.timeout < minimumTimeout || commandOptions.timeout > maximumTimeout {
		fmt.Fprintln(stderr, "status: --timeout muss zwischen 100ms und 1m liegen")
		return diagnostic.ExitFailure
	}
	baseURL, err := apiinspect.NormalizeBaseURL(commandOptions.baseURL)
	if err != nil {
		fmt.Fprintf(stderr, "status: %v\n", err)
		return diagnostic.ExitFailure
	}
	if getter == nil {
		getter = apiinspect.NewHTTPGetter(commandOptions.timeout)
	}
	report := diagnostic.NewReport(
		"status",
		apiinspect.PublicChecks(ctx, getter, baseURL, commandOptions.timeout),
	)
	if err := report.Write(stdout, commandOptions.json); err != nil {
		fmt.Fprintln(stderr, "status: Bericht konnte nicht geschrieben werden")
		return diagnostic.ExitFailure
	}
	return report.ExitCode()
}
