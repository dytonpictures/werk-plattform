// Package regression contains the permanent black-box developer checks for a
// running WERK API. It intentionally lives outside production packages: the
// checks exercise public HTTP contracts without scattering *_test.go files
// throughout the application.
package regression

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dytonpictures/werk/cmd/werkctl/internal/apiinspect"
	"github.com/dytonpictures/werk/cmd/werkctl/internal/diagnostic"
)

const (
	defaultTimeout = 5 * time.Second
	minimumTimeout = 100 * time.Millisecond
	maximumTimeout = time.Minute
)

// Command runs the stable, unauthenticated black-box regression suite.
// Authentication and destructive workflows belong in a separately provisioned
// disposable environment; this command never mutates the target.
func Command(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	flags := flag.NewFlagSet("regression", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var baseURL string
	var jsonOutput bool
	var timeout time.Duration
	flags.StringVar(&baseURL, "url", "", "explicit WERK API base URL (required)")
	flags.BoolVar(&jsonOutput, "json", false, "write the stable JSON report")
	flags.DurationVar(&timeout, "timeout", defaultTimeout, "timeout per request")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: werkctl regression --url URL [--json] [--timeout DURATION]")
	}
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return diagnostic.ExitHealthy
		}
		return diagnostic.ExitFailure
	}
	if flags.NArg() != 0 || timeout < minimumTimeout || timeout > maximumTimeout {
		flags.Usage()
		return diagnostic.ExitFailure
	}
	normalized, err := apiinspect.NormalizeBaseURL(baseURL)
	if err != nil {
		fmt.Fprintf(stderr, "regression: %v\n", err)
		return diagnostic.ExitFailure
	}
	getter := apiinspect.NewHTTPGetter(timeout)
	checks := append([]diagnostic.Check(nil), apiinspect.PublicChecks(ctx, getter, normalized, timeout)...)
	checks = append(checks,
		checkPublicPage(ctx, getter, normalized+"/", timeout, "ui.home", "Startseite"),
		checkPublicPage(ctx, getter, normalized+"/admin", timeout, "ui.admin", "Administrationsoberfläche"),
		checkPublicAsset(ctx, normalized+"/admin.js", timeout, "ui.admin-script", "Admin-JavaScript"),
	)
	report := diagnostic.NewReport("regression", checks)
	if err := report.Write(stdout, jsonOutput); err != nil {
		fmt.Fprintln(stderr, "regression: Bericht konnte nicht geschrieben werden")
		return diagnostic.ExitFailure
	}
	return report.ExitCode()
}

func checkPublicAsset(ctx context.Context, endpoint string, timeout time.Duration, id, name string) diagnostic.Check {
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return diagnostic.Check{ID: id, Status: diagnostic.Fail, Summary: name + " Anfrage ist ungültig", Diagnosis: "Die intern gebildete Asset-URL konnte nicht verwendet werden."}
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return diagnostic.Check{ID: id, Status: diagnostic.Fail, Summary: name + " nicht erreichbar", Diagnosis: "HTTP-Verbindung oder TLS-Prüfung fehlgeschlagen.", Remediation: "API-URL, Listener und TLS-Konfiguration prüfen."}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 256<<10))
	if err != nil || len(strings.TrimSpace(string(body))) == 0 || response.StatusCode != http.StatusOK {
		return diagnostic.Check{ID: id, Status: diagnostic.Fail, Summary: name + " liefert keine gültige Antwort", Diagnosis: fmt.Sprintf("HTTP %d oder leerer Asset-Inhalt", response.StatusCode), Remediation: "Embedded Dashboard-Assets und Routing prüfen."}
	}
	return diagnostic.Check{ID: id, Status: diagnostic.Pass, Summary: name + " ist erreichbar"}
}

func checkPublicPage(ctx context.Context, getter apiinspect.Getter, endpoint string, timeout time.Duration, id, name string) diagnostic.Check {
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := getter.Get(requestContext, endpoint)
	if err != nil {
		return diagnostic.Check{ID: id, Status: diagnostic.Fail, Summary: name + " nicht erreichbar", Diagnosis: err.Error(), Remediation: "API-URL, Listener und TLS-Konfiguration prüfen."}
	}
	if response.StatusCode != http.StatusOK {
		return diagnostic.Check{ID: id, Status: diagnostic.Fail, Summary: name + " liefert unerwarteten HTTP-Status", Diagnosis: fmt.Sprintf("HTTP %d für %s", response.StatusCode, endpoint), Remediation: "Deployment und öffentliche Routen prüfen."}
	}
	if len(strings.TrimSpace(string(response.Body))) == 0 {
		return diagnostic.Check{ID: id, Status: diagnostic.Fail, Summary: name + " liefert eine leere Antwort", Remediation: "Embedded Dashboard-Assets und Routing prüfen."}
	}
	return diagnostic.Check{ID: id, Status: diagnostic.Pass, Summary: name + " ist erreichbar"}
}
