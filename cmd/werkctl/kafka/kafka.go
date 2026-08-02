// Package kafka implements the explicit, executable Kafka transport proof.
// It verifies broker reachability and every configured durable topic without
// producing messages or enabling broker-side auto-creation.
package kafka

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/dytonpictures/werk/cmd/werkctl/internal/diagnostic"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/envfile"
	"github.com/dytonpictures/werk/internal/platform/kafkastream"
)

func Command(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	flags := flag.NewFlagSet("kafka-regression", flag.ContinueOnError)
	flags.SetOutput(stderr)
	envPath := flags.String("env", ".env", "shared WERK environment file")
	jsonOutput := flags.Bool("json", false, "write the stable JSON report")
	timeout := flags.Duration("timeout", 5*time.Second, "timeout for the broker proof")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return diagnostic.ExitHealthy
		}
		return diagnostic.ExitFailure
	}
	if flags.NArg() != 0 || *timeout < 100*time.Millisecond || *timeout > time.Minute {
		fmt.Fprintln(stderr, "usage: werkctl kafka-regression [--env PATH] [--json] [--timeout DURATION]")
		return diagnostic.ExitFailure
	}
	values, err := envfile.LoadSecure(*envPath)
	if err != nil {
		return writeFailure(stdout, *jsonOutput, "config.environment", "Umgebungsdatei konnte nicht gelesen werden", err.Error())
	}
	worker, err := config.LoadWorkerFromMap(values)
	if err != nil {
		return writeFailure(stdout, *jsonOutput, "config.kafka", "Worker-/Kafka-Konfiguration ist ungültig", err.Error())
	}
	checks := make([]diagnostic.Check, 0, 2)
	if !worker.Kafka.Enabled {
		checks = append(checks, diagnostic.Check{ID: "kafka.enabled", Status: diagnostic.Warn, Summary: "Kafka ist deaktiviert", Diagnosis: "Es wurde kein Broker- und Topic-Nachweis ausgeführt.", Remediation: "Kafka aktivieren, wenn Ereignis- und Audit-Streams betrieben werden sollen."})
	} else {
		checks = append(checks, diagnostic.Check{ID: "kafka.enabled", Status: diagnostic.Pass, Summary: "Kafka ist aktiviert"})
		checkContext, cancel := context.WithTimeout(ctx, *timeout)
		client, clientErr := kafkastream.NewClient(worker.Kafka)
		if clientErr != nil {
			checks = append(checks, diagnostic.Check{ID: "kafka.transport", Status: diagnostic.Fail, Summary: "Kafka-Client konnte nicht erstellt werden", Diagnosis: clientErr.Error(), Remediation: "Broker, TLS, SASL und Topic-Konfiguration prüfen."})
		} else {
			defer client.Close()
			proofErr := client.VerifyTopics(checkContext)
			if proofErr != nil {
				checks = append(checks, diagnostic.Check{ID: "kafka.transport", Status: diagnostic.Fail, Summary: "Kafka-Broker- und Topic-Nachweis fehlgeschlagen", Diagnosis: proofErr.Error(), Remediation: "Broker, TLS/SASL, ACLs und die drei konfigurierten Topics prüfen."})
			} else {
				checks = append(checks, diagnostic.Check{ID: "kafka.transport", Status: diagnostic.Pass, Summary: "Broker erreichbar und alle drei Topics vorhanden"})
			}
		}
		cancel()
	}
	report := diagnostic.NewReport("kafka-regression", checks)
	if err := report.Write(stdout, *jsonOutput); err != nil {
		return diagnostic.ExitFailure
	}
	return report.ExitCode()
}

func writeFailure(stdout io.Writer, jsonOutput bool, id, summary, diagnosis string) int {
	report := diagnostic.NewReport("kafka-regression", []diagnostic.Check{{ID: id, Status: diagnostic.Fail, Summary: summary, Diagnosis: diagnosis}})
	_ = report.Write(stdout, jsonOutput)
	return report.ExitCode()
}
