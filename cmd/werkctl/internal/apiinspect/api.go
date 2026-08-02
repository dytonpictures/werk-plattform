// Package apiinspect implements bounded, unauthenticated reads of WERK's
// public operational endpoints. It deliberately knows nothing about admin
// sessions, hosts, process managers, containers or deployment systems.
package apiinspect

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dytonpictures/werk/cmd/werkctl/internal/diagnostic"
)

const maximumResponseBytes = 64 << 10

// Response contains only the bounded information needed to validate a public
// endpoint contract.
type Response struct {
	StatusCode int
	Body       []byte
}

// Getter is injectable so command tests never need a real network listener.
type Getter interface {
	Get(context.Context, string) (Response, error)
}

// HTTPGetter performs GET requests without cookies, authentication or
// redirects. Its client retains the normal system TLS verification behavior.
type HTTPGetter struct {
	client *http.Client
}

func NewHTTPGetter(timeout time.Duration) *HTTPGetter {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &HTTPGetter{client: &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (getter *HTTPGetter) Get(ctx context.Context, endpoint string) (Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Response{}, errors.New("HTTP-Anfrage ist ungültig")
	}
	request.Header.Set("Accept", "application/json")
	response, err := getter.client.Do(request)
	if err != nil {
		return Response{}, safeRequestError(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return Response{}, errors.New("HTTP-Antwort konnte nicht vollständig gelesen werden")
	}
	if len(body) > maximumResponseBytes {
		return Response{}, fmt.Errorf("HTTP-Antwort überschreitet %d Byte", maximumResponseBytes)
	}
	return Response{StatusCode: response.StatusCode, Body: body}, nil
}

func safeRequestError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("Zeitlimit der HTTP-Anfrage überschritten: %w", context.DeadlineExceeded)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("HTTP-Anfrage abgebrochen: %w", context.Canceled)
	}
	return errors.New("HTTP-Verbindung oder TLS-Prüfung fehlgeschlagen")
}

// NormalizeBaseURL validates an operator-supplied target without performing
// DNS or network access. Listener addresses are intentionally not accepted as
// substitutes: bind addresses are not canonical dial or TLS targets.
func NormalizeBaseURL(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", errors.New("--url ist erforderlich")
	}
	parsed, err := url.Parse(configured)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", errors.New("--url muss eine absolute HTTP- oder HTTPS-URL sein")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("--url unterstützt nur http oder https")
	}
	if parsed.User != nil {
		return "", errors.New("--url darf keine Zugangsdaten enthalten")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return "", errors.New("--url darf weder Query noch Fragment enthalten")
	}
	if parsed.Path != "" && parsed.Path != "/" || parsed.RawPath != "" {
		return "", errors.New("--url darf keinen Basispfad enthalten")
	}
	if parsed.Hostname() == "" {
		return "", errors.New("--url muss einen Hostnamen enthalten")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return "", errors.New("HTTP ohne TLS ist nur für ein Loopback-Ziel zulässig")
	}
	parsed.Path = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

// PublicChecks validates the exact public API contracts. Readiness proves the
// API's own readiness contract, not worker, Kafka, migration or host status.
func PublicChecks(ctx context.Context, getter Getter, baseURL string, requestTimeout time.Duration) []diagnostic.Check {
	checks := make([]diagnostic.Check, 3)
	var wait sync.WaitGroup
	wait.Add(3)
	go func() {
		defer wait.Done()
		checks[0] = checkMetadata(ctx, getter, baseURL+"/meta", requestTimeout)
	}()
	go func() {
		defer wait.Done()
		checks[1] = checkHealth(ctx, getter, baseURL+"/health/live", requestTimeout, "api.live", "ok")
	}()
	go func() {
		defer wait.Done()
		checks[2] = checkHealth(ctx, getter, baseURL+"/health/ready", requestTimeout, "api.ready", "ready")
	}()
	wait.Wait()
	if checks[0].Status != diagnostic.Pass {
		for index := 1; index < len(checks); index++ {
			if checks[index].Status != diagnostic.Pass {
				continue
			}
			checks[index] = diagnostic.Check{
				ID: checks[index].ID, Status: diagnostic.Fail,
				Summary:     "Health-Antwort ist formal gültig, aber das Ziel nicht als WERK bestätigt",
				Diagnosis:   "api.meta hat den WERK-Metadatenvertrag nicht bestätigt.",
				Remediation: "Explizite Ziel-URL und installierte WERK-Version prüfen.",
			}
		}
	}
	return checks
}

type healthResponse struct {
	Status string `json:"status"`
}

func checkHealth(
	ctx context.Context,
	getter Getter,
	endpoint string,
	timeout time.Duration,
	id string,
	expected string,
) diagnostic.Check {
	response, err := getWithTimeout(ctx, getter, endpoint, timeout)
	if err != nil {
		return diagnostic.Check{
			ID: id, Status: diagnostic.Fail, Summary: "Öffentlicher API-Check nicht erreichbar",
			Diagnosis: err.Error(), Remediation: "API-Ziel, Netzwerk und TLS-Vertrauenskette prüfen.",
		}
	}
	if response.StatusCode != http.StatusOK {
		diagnosis := fmt.Sprintf("Unerwarteter HTTP-Status %d.", response.StatusCode)
		if id == "api.ready" && response.StatusCode == http.StatusServiceUnavailable {
			diagnosis = "HTTP 503: Die API meldet eine erforderliche Abhängigkeit als nicht bereit."
		}
		return diagnostic.Check{
			ID: id, Status: diagnostic.Fail, Summary: "Öffentlicher API-Check ist negativ",
			Diagnosis: diagnosis, Remediation: readinessRemediation(id),
		}
	}
	var payload healthResponse
	if err := decodeExactJSON(response.Body, &payload); err != nil || payload.Status != expected {
		return diagnostic.Check{
			ID: id, Status: diagnostic.Fail, Summary: "API-Antwort verletzt den Health-Vertrag",
			Diagnosis:   "Die Antwort enthält nicht den erwarteten Status.",
			Remediation: "Ziel-URL und API-Version prüfen; keine Proxy-Fehlerseite verwenden.",
		}
	}
	if id == "api.live" {
		return diagnostic.Check{ID: id, Status: diagnostic.Pass, Summary: "Der WERK-HTTP-Prozess antwortet"}
	}
	return diagnostic.Check{
		ID: id, Status: diagnostic.Pass,
		Summary: "Die WERK-API meldet ihren öffentlichen Readiness-Vertrag als bereit",
	}
}

func readinessRemediation(id string) string {
	if id == "api.ready" {
		return "API- und Work-PostgreSQL-Zustand prüfen; der Check belegt keinen Worker- oder Kafka-Status."
	}
	return "API-Prozess und Ziel-URL prüfen."
}

type metadataResponse struct {
	Product    string `json:"product"`
	Service    string `json:"service"`
	Version    string `json:"version"`
	APIVersion string `json:"api_version"`
}

func checkMetadata(ctx context.Context, getter Getter, endpoint string, timeout time.Duration) diagnostic.Check {
	response, err := getWithTimeout(ctx, getter, endpoint, timeout)
	if err != nil {
		return diagnostic.Check{
			ID: "api.meta", Status: diagnostic.Fail, Summary: "API-Ziel konnte nicht als WERK erkannt werden",
			Diagnosis: err.Error(), Remediation: "Explizite --url, Netzwerk und TLS-Vertrauenskette prüfen.",
		}
	}
	if response.StatusCode != http.StatusOK {
		return diagnostic.Check{
			ID: "api.meta", Status: diagnostic.Fail, Summary: "API-Ziel konnte nicht als WERK erkannt werden",
			Diagnosis:   fmt.Sprintf("Metadaten-Endpunkt antwortet mit HTTP %d.", response.StatusCode),
			Remediation: "Basis-URL ohne Pfad angeben und Zielversion prüfen.",
		}
	}
	var metadata metadataResponse
	if err := decodeExactJSON(response.Body, &metadata); err != nil ||
		metadata.Product != "WERK" || metadata.Service != "werk-api" ||
		metadata.APIVersion != "v1" || strings.TrimSpace(metadata.Version) == "" {
		return diagnostic.Check{
			ID: "api.meta", Status: diagnostic.Fail, Summary: "API-Ziel verletzt den WERK-Metadatenvertrag",
			Diagnosis:   "Produkt, Dienst, Version oder API-Version stimmen nicht mit WERK v1 überein.",
			Remediation: "Ziel-URL und installierte WERK-Version prüfen.",
		}
	}
	return diagnostic.Check{
		ID: "api.meta", Status: diagnostic.Pass,
		Summary: "WERK API v1 erkannt (Build " + safeLabel(metadata.Version) + ")",
	}
}

func getWithTimeout(ctx context.Context, getter Getter, endpoint string, timeout time.Duration) (Response, error) {
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := getter.Get(requestContext, endpoint)
	if err == nil {
		return response, nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestContext.Err(), context.DeadlineExceeded) {
		return Response{}, errors.New("Zeitlimit der HTTP-Anfrage überschritten")
	}
	if errors.Is(err, context.Canceled) || errors.Is(requestContext.Err(), context.Canceled) {
		return Response{}, errors.New("HTTP-Anfrage abgebrochen")
	}
	return Response{}, errors.New("HTTP-Verbindung oder TLS-Prüfung fehlgeschlagen")
}

func decodeExactJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("response contains more than one JSON value")
	}
	return nil
}

func safeLabel(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 128 {
		value = string(runes[:128])
	}
	return strconv.QuoteToASCII(value)
}
