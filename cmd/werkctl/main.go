package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dytonpictures/werk/cmd/werkctl/doctor"
	"github.com/dytonpictures/werk/cmd/werkctl/internal/safeoutput"
	"github.com/dytonpictures/werk/cmd/werkctl/kafka"
	"github.com/dytonpictures/werk/cmd/werkctl/regression"
	"github.com/dytonpictures/werk/cmd/werkctl/status"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/envfile"
	"github.com/dytonpictures/werk/internal/platform/migrate"
)

var version = "dev"

const (
	exitUsage      = 2
	exitConfig     = 3
	exitDependency = 4
	exitMigration  = 5
)

type commandMode string

const (
	modeReadOnly commandMode = "read-only"
	modeMutation commandMode = "mutation"
)

// commandRunner is the typed dispatch boundary. The concrete wrapper owns its
// mode so help metadata cannot drift independently from the registered runner.
// Authorization of mutations remains the responsibility of a dedicated
// implementation; this type is not a security sandbox.
type commandRunner interface {
	Run(context.Context, []string, io.Writer, io.Writer) int
	Mode() commandMode
}

type readOnlyRunner func(context.Context, []string, io.Writer, io.Writer) int

func (runner readOnlyRunner) Run(
	ctx context.Context,
	arguments []string,
	stdout, stderr io.Writer,
) int {
	return runner(ctx, arguments, stdout, stderr)
}

func (readOnlyRunner) Mode() commandMode { return modeReadOnly }

type mutationRunner func(context.Context, []string, io.Writer, io.Writer) int

func (runner mutationRunner) Run(
	ctx context.Context,
	arguments []string,
	stdout, stderr io.Writer,
) int {
	return runner(ctx, arguments, stdout, stderr)
}

func (mutationRunner) Mode() commandMode { return modeMutation }

type commandDefinition struct {
	Name     string
	Synopsis string
	Summary  string
	Runner   commandRunner
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	exitCode := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(exitCode)
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(arguments) == 0 {
		printUsage(stderr)
		return exitUsage
	}
	if arguments[0] == "help" || arguments[0] == "-h" || arguments[0] == "--help" {
		printUsage(stdout)
		return 0
	}
	for _, definition := range commandDefinitions() {
		if arguments[0] == definition.Name {
			return definition.Runner.Run(ctx, arguments[1:], stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "unknown command %q\n", arguments[0])
	printUsage(stderr)
	return exitUsage
}

func commandDefinitions() []commandDefinition {
	return []commandDefinition{
		{
			Name: "version", Synopsis: "werkctl version", Summary: "Build-Version ausgeben",
			Runner: readOnlyRunner(func(_ context.Context, arguments []string, stdout, stderr io.Writer) int {
				if len(arguments) != 0 {
					fmt.Fprintln(stderr, "usage: werkctl version")
					return exitUsage
				}
				fmt.Fprintf(stdout, "werkctl %s\n", version)
				return 0
			}),
		},
		{
			Name:     "doctor",
			Synopsis: "werkctl doctor [--env PATH] [--url URL] [--config-only] [--json] [--timeout DURATION]",
			Summary:  "Konfiguration und explizit gewählte Abhängigkeiten inventarisieren",
			Runner:   readOnlyRunner(doctor.Command),
		},
		{
			Name: "status", Synopsis: "werkctl status --url URL [--json] [--timeout DURATION]",
			Summary: "Nur öffentliche WERK-API-Metadaten, Liveness und Readiness lesen",
			Runner:  readOnlyRunner(status.Command),
		},
		{
			Name: "regression", Synopsis: "werkctl regression --url URL [--json] [--timeout DURATION]",
			Summary: "Dauerhafte, nicht mutierende Blackbox-Regressionsprüfungen ausführen",
			Runner:  readOnlyRunner(regression.Command),
		},
		{
			Name: "kafka-regression", Synopsis: "werkctl kafka-regression [--env PATH] [--json] [--timeout DURATION]",
			Summary: "Kafka-Broker und durable WERK-Topics ausführbar nachweisen",
			Runner:  readOnlyRunner(kafka.Command),
		},
		{
			Name: "migrate", Synopsis: "werkctl migrate [--env PATH]",
			Summary: "Versionierte Datenbankmigrationen anwenden (bestehender Mutationsbefehl)",
			Runner:  mutationRunner(migrateCommand),
		},
	}
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage:")
	for _, definition := range commandDefinitions() {
		fmt.Fprintf(writer, "  %-88s [%s]\n", definition.Synopsis, definition.Runner.Mode())
		fmt.Fprintf(writer, "      %s\n", definition.Summary)
	}
}

func migrateCommand(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("env", ".env", "shared WERK environment file")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	if flags.NArg() != 0 {
		return exitUsage
	}
	values, err := envfile.LoadSecure(*path)
	if err != nil {
		fmt.Fprintf(stderr, "migration environment invalid: %v\n", err)
		return exitConfig
	}
	if err := envfile.ValidateConfigured(values); err != nil {
		fmt.Fprintf(stderr, "migration environment invalid: %v\n", err)
		return exitConfig
	}
	configuration, err := config.LoadMigrationFromMap(values)
	if err != nil {
		fmt.Fprintf(stderr, "migration configuration invalid: %s\n", safeoutput.Redact(err.Error(), values))
		return exitConfig
	}
	migrationContext, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	pool, err := database.NewMigrationPool(migrationContext, configuration.DatabaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "migration database unavailable: %s\n", safeoutput.Redact(err.Error(), values))
		return exitDependency
	}
	defer pool.Close()
	if err := migrate.Apply(migrationContext, pool); err != nil {
		fmt.Fprintf(stderr, "migration failed: %s\n", safeoutput.Redact(err.Error(), values))
		return exitMigration
	}
	fmt.Fprintln(stdout, "OK migrations complete")
	return 0
}
