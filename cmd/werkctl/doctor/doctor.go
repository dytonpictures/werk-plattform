// Package doctor implements the read-only WERK installation inventory.
package doctor

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dytonpictures/werk/cmd/werkctl/internal/apiinspect"
	"github.com/dytonpictures/werk/cmd/werkctl/internal/diagnostic"
	"github.com/dytonpictures/werk/cmd/werkctl/internal/safeoutput"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/envfile"
	"github.com/dytonpictures/werk/internal/platform/kafkastream"
	"github.com/dytonpictures/werk/internal/platform/transportsecurity"
)

const (
	ExitSuccess = diagnostic.ExitHealthy
	ExitWarning = diagnostic.ExitWarning
	ExitFailure = diagnostic.ExitFailure
	ExitUsage   = diagnostic.ExitFailure
)

const (
	defaultProbeTimeout = 5 * time.Second
	minimumProbeTimeout = 100 * time.Millisecond
	maximumProbeTimeout = time.Minute
	migrationRole       = "werk_migrator"
)

type options struct {
	environmentFile string
	baseURL         string
	configOnly      bool
	json            bool
	timeout         time.Duration
}

type databasePurpose string

const (
	databaseWork      databasePurpose = "work"
	databaseIdentity  databasePurpose = "identity"
	databaseAdmin     databasePurpose = "admin"
	databaseWorker    databasePurpose = "worker"
	databaseMigration databasePurpose = "migration"
)

// prober is the read-only runtime boundary. A later mutating runner must not
// be added to this interface: operations need a separate authorization and
// execution contract.
type prober interface {
	apiinspect.Getter
	CheckDatabase(context.Context, databasePurpose, string) error
	CheckKafka(context.Context, config.KafkaConfig) error
}

type runtimeProber struct {
	http *apiinspect.HTTPGetter
}

// Command inventories a WERK installation and returns the stable diagnostic
// exit contract: 0 healthy, 1 warnings, 2 failures or invalid use.
func Command(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	return commandWithProber(ctx, arguments, stdout, stderr, nil)
}

func commandWithProber(
	ctx context.Context,
	arguments []string,
	stdout, stderr io.Writer,
	prober prober,
) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	commandOptions := options{environmentFile: ".env", timeout: defaultProbeTimeout}
	flags.StringVar(&commandOptions.environmentFile, "env", ".env", "shared WERK environment file")
	flags.StringVar(&commandOptions.baseURL, "url", "", "explicit WERK API base URL to probe")
	flags.BoolVar(&commandOptions.configOnly, "config-only", false, "validate configuration without connecting")
	flags.BoolVar(&commandOptions.json, "json", false, "write the stable JSON report")
	flags.DurationVar(&commandOptions.timeout, "timeout", defaultProbeTimeout, "timeout per dependency probe")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: werkctl doctor [--env PATH] [--url URL] [--config-only] [--json] [--timeout DURATION]")
	}
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitSuccess
		}
		return ExitUsage
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return ExitUsage
	}
	if commandOptions.timeout < minimumProbeTimeout || commandOptions.timeout > maximumProbeTimeout {
		fmt.Fprintln(stderr, "doctor: --timeout muss zwischen 100ms und 1m liegen")
		return ExitUsage
	}
	if commandOptions.baseURL != "" {
		baseURL, err := apiinspect.NormalizeBaseURL(commandOptions.baseURL)
		if err != nil {
			fmt.Fprintf(stderr, "doctor: %v\n", err)
			return ExitUsage
		}
		commandOptions.baseURL = baseURL
	}
	if prober == nil {
		prober = &runtimeProber{http: apiinspect.NewHTTPGetter(commandOptions.timeout)}
	}

	report := runDoctor(ctx, commandOptions, prober)
	if err := report.Write(stdout, commandOptions.json); err != nil {
		fmt.Fprintln(stderr, "doctor: Bericht konnte nicht geschrieben werden")
		return ExitFailure
	}
	return report.ExitCode()
}

func runDoctor(ctx context.Context, commandOptions options, prober prober) diagnostic.Report {
	checks := make([]diagnostic.Check, 0, 20)
	values, err := envfile.LoadSecure(commandOptions.environmentFile)
	if err != nil {
		checks = append(checks, diagnostic.Check{
			ID: "config.environment-file", Status: diagnostic.Fail,
			Summary:     "Umgebungsdatei ist nicht sicher lesbar",
			Diagnosis:   safeoutput.Redact(err.Error(), nil),
			Remediation: "Pfad, reguläre Datei und restriktive Dateirechte prüfen.",
		})
		return diagnostic.NewReport("doctor", checks)
	}
	checks = append(checks, diagnostic.Check{
		ID: "config.environment-file", Status: diagnostic.Pass,
		Summary: "Umgebungsdatei ist regulär, begrenzt und restriktiv lesbar",
	})
	if err := envfile.ValidateConfigured(values); err != nil {
		checks = append(checks, diagnostic.Check{
			ID: "config.template-values", Status: diagnostic.Fail,
			Summary:     "Konfiguration enthält unveränderte Vorlagenwerte",
			Diagnosis:   safeoutput.Redact(err.Error(), values),
			Remediation: "Jeden genannten CHANGE_ME-Wert ersetzen.",
		})
		return diagnostic.NewReport("doctor", checks)
	}
	checks = append(checks, diagnostic.Check{
		ID: "config.template-values", Status: diagnostic.Pass,
		Summary: "Keine unveränderten CHANGE_ME-Werte gefunden",
	})

	environment := strings.TrimSpace(values["WERK_ENV"])
	if environment == "" {
		environment = "development"
	}
	checks = append(checks, requiredConfigurationCheck(values, environment))

	apiConfiguration, apiOK, apiCheck := loadAPIConfiguration(values)
	checks = append(checks, apiCheck)
	workerConfiguration, workerOK, workerCheck := loadWorkerConfiguration(values)
	checks = append(checks, workerCheck)
	migrationConfiguration, migrationOK, migrationCheck := loadMigrationConfiguration(values)
	checks = append(checks, migrationCheck)

	checks = append(checks, listenerCheck(apiConfiguration, apiOK))
	checks = append(checks, tlsMaterialCheck(apiConfiguration, apiOK))
	databaseDefinitions := resolvedDatabaseDefinitions(
		apiConfiguration,
		apiOK,
		workerConfiguration,
		workerOK,
		migrationConfiguration,
		migrationOK,
	)
	for _, definition := range databaseDefinitions {
		checks = append(checks, databaseStructureCheck(definition))
	}
	checks = append(checks, databaseSeparationCheck(databaseDefinitions))

	configurationFailed := containsFailure(checks)
	if commandOptions.configOnly || configurationFailed {
		return diagnostic.NewReport("doctor", checks)
	}
	checks = append(checks, runtimeChecks(
		ctx,
		prober,
		databaseDefinitions,
		workerConfiguration.Kafka,
		commandOptions.baseURL,
		commandOptions.timeout,
	)...)
	return diagnostic.NewReport("doctor", checks)
}

func requiredConfigurationCheck(values map[string]string, environment string) diagnostic.Check {
	required := []string{
		"WERK_ENV",
		"WERK_HTTP_ADDRESS",
		"WERK_HTTP_TLS_MODE",
		"WORK_DATABASE_URL",
		"IDENTITY_DATABASE_URL",
		"ADMIN_DATABASE_URL",
		"WORKER_DATABASE_URL",
		"MIGRATOR_DATABASE_URL",
	}
	missing := make([]string, 0)
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return diagnostic.Check{
			ID: "config.required", Status: diagnostic.Pass,
			Summary: "Native Basiswerte und alle zweckgebundenen Datenbank-URLs sind explizit gesetzt",
		}
	}
	status := diagnostic.Warn
	if environment == "production" {
		status = diagnostic.Fail
	}
	return diagnostic.Check{
		ID: "config.required", Status: status,
		Summary:     "Native Basiswerte sind nicht vollständig explizit gesetzt",
		Diagnosis:   "Fehlende Schlüssel: " + strings.Join(missing, ", ") + ".",
		Remediation: "Die genannten Werte zweckgebunden setzen; keine gemeinsame DATABASE_URL als Laufzeitabkürzung verwenden.",
	}
}

func loadAPIConfiguration(values map[string]string) (config.Config, bool, diagnostic.Check) {
	configuration, err := config.LoadAPIFromMap(values)
	if err != nil {
		return config.Config{}, false, diagnostic.Check{
			ID: "config.api", Status: diagnostic.Fail, Summary: "API-Konfiguration ist ungültig",
			Diagnosis:   safeoutput.Redact(err.Error(), values),
			Remediation: "API-, Browser-, Identitäts- und Transportwerte prüfen.",
		}
	}
	return configuration, true, diagnostic.Check{
		ID: "config.api", Status: diagnostic.Pass, Summary: "API-Konfiguration erfüllt den Core-Vertrag",
	}
}

func loadWorkerConfiguration(values map[string]string) (config.WorkerConfig, bool, diagnostic.Check) {
	configuration, err := config.LoadWorkerFromMap(values)
	if err != nil {
		return config.WorkerConfig{}, false, diagnostic.Check{
			ID: "config.worker", Status: diagnostic.Fail, Summary: "Worker-Konfiguration ist ungültig",
			Diagnosis:   safeoutput.Redact(err.Error(), values),
			Remediation: "Worker-Datenbank-, Parallelitäts- und Kafka-Werte prüfen.",
		}
	}
	return configuration, true, diagnostic.Check{
		ID: "config.worker", Status: diagnostic.Pass, Summary: "Worker-Konfiguration erfüllt den Core-Vertrag",
	}
}

func loadMigrationConfiguration(values map[string]string) (config.MigrationConfig, bool, diagnostic.Check) {
	configuration, err := config.LoadMigrationFromMap(values)
	if err != nil {
		return config.MigrationConfig{}, false, diagnostic.Check{
			ID: "config.migration", Status: diagnostic.Fail, Summary: "Migrationskonfiguration ist ungültig",
			Diagnosis:   safeoutput.Redact(err.Error(), values),
			Remediation: "Die ausschließlich für Migrationen bestimmte Datenbank-URL prüfen.",
		}
	}
	return configuration, true, diagnostic.Check{
		ID: "config.migration", Status: diagnostic.Pass, Summary: "Migrationskonfiguration erfüllt den Core-Vertrag",
	}
}

func listenerCheck(configuration config.Config, available bool) diagnostic.Check {
	check := diagnostic.Check{ID: "http.listener"}
	if !available {
		check.Status = diagnostic.Fail
		check.Summary = "Listener-Sicherheit kann nicht geprüft werden"
		check.Diagnosis = "Die API-Konfiguration ist ungültig."
		check.Remediation = "Zuerst config.api beheben."
		return check
	}
	if err := validateDirectDevelopmentListener(configuration); err != nil {
		check.Status = diagnostic.Fail
		check.Summary = "HTTP-Listener verletzt die Transportgrenze"
		check.Diagnosis = "Entwicklung oder Test ohne TLS bindet nicht ausschließlich an Loopback."
		check.Remediation = "Loopback verwenden oder den Listener mit TLS absichern."
		return check
	}
	check.Status = diagnostic.Pass
	if configuration.HTTPServerTLS.Enabled() {
		check.Summary = "Listener ist durch TLS abgesichert"
	} else {
		check.Summary = "HTTP ohne TLS ist auf Loopback begrenzt"
	}
	return check
}

func tlsMaterialCheck(configuration config.Config, available bool) diagnostic.Check {
	check := diagnostic.Check{ID: "http.tls-material"}
	if !available {
		check.Status = diagnostic.Fail
		check.Summary = "TLS-Material kann nicht geprüft werden"
		check.Diagnosis = "Die API-Konfiguration ist ungültig."
		check.Remediation = "Zuerst config.api beheben."
		return check
	}
	if !configuration.HTTPServerTLS.Enabled() {
		check.Status = diagnostic.Pass
		check.Summary = "Kein Server-TLS-Material für den zulässigen Loopback-Modus benötigt"
		return check
	}
	if _, err := transportsecurity.NewServerTLSConfig(configuration.HTTPServerTLS); err != nil {
		check.Status = diagnostic.Fail
		check.Summary = "Server-TLS-Material ist nicht verwendbar"
		check.Diagnosis = "Zertifikat, privater Schlüssel oder Client-CA sind nicht lesbar oder ungültig."
		check.Remediation = "Dateirechte, Zertifikatskette, Schlüsselzuordnung und Gültigkeit prüfen."
		return check
	}
	check.Status = diagnostic.Pass
	check.Summary = "Server-TLS-Material ist lesbar und kryptografisch verwendbar"
	return check
}

type databaseDefinition struct {
	ID           string
	Key          string
	Purpose      databasePurpose
	URL          string
	ExpectedRole string
	Available    bool
}

func resolvedDatabaseDefinitions(
	apiConfiguration config.Config,
	apiOK bool,
	workerConfiguration config.WorkerConfig,
	workerOK bool,
	migrationConfiguration config.MigrationConfig,
	migrationOK bool,
) []databaseDefinition {
	return []databaseDefinition{
		{ID: "database.dsn.work", Key: "WORK_DATABASE_URL", Purpose: databaseWork, URL: apiConfiguration.DatabaseURL, ExpectedRole: database.WorkRuntimeRole, Available: apiOK},
		{ID: "database.dsn.identity", Key: "IDENTITY_DATABASE_URL", Purpose: databaseIdentity, URL: apiConfiguration.IdentityDatabaseURL, ExpectedRole: database.IdentityRuntimeRole, Available: apiOK},
		{ID: "database.dsn.admin", Key: "ADMIN_DATABASE_URL", Purpose: databaseAdmin, URL: apiConfiguration.AdminDatabaseURL, ExpectedRole: database.AdminRuntimeRole, Available: apiOK},
		{ID: "database.dsn.worker", Key: "WORKER_DATABASE_URL", Purpose: databaseWorker, URL: workerConfiguration.DatabaseURL, ExpectedRole: database.WorkerRuntimeRole, Available: workerOK},
		{ID: "database.dsn.migrator", Key: "MIGRATOR_DATABASE_URL", Purpose: databaseMigration, URL: migrationConfiguration.DatabaseURL, ExpectedRole: migrationRole, Available: migrationOK},
	}
}

func databaseStructureCheck(definition databaseDefinition) diagnostic.Check {
	check := diagnostic.Check{ID: definition.ID}
	if !definition.Available {
		check.Status = diagnostic.Fail
		check.Summary = "Datenbank-URL kann nicht strukturell geprüft werden"
		check.Diagnosis = "Die zugehörige Prozesskonfiguration ist ungültig."
		check.Remediation = "Zuerst die zugehörige config.*-Prüfung beheben."
		return check
	}
	parsed, err := url.Parse(definition.URL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		strings.Trim(parsed.Path, "/") == "" || (parsed.Hostname() == "" && parsed.Query().Get("host") == "") {
		check.Status = diagnostic.Fail
		check.Summary = "Datenbank-URL hat keine sichere PostgreSQL-Struktur"
		check.Diagnosis = definition.Key + " benötigt PostgreSQL-Schema, Zielserver oder Socket und Datenbankname."
		check.Remediation = "Nur eine vollständige zweckgebundene PostgreSQL-URL konfigurieren."
		return check
	}
	if parsed.User == nil || parsed.User.Username() != definition.ExpectedRole {
		check.Status = diagnostic.Fail
		check.Summary = "Datenbank-URL verwendet nicht die erwartete zweckgebundene Rolle"
		check.Diagnosis = definition.Key + " ist keiner passenden Core-Laufzeitrolle zugeordnet."
		check.Remediation = "Die für diesen Prozess vorgesehene WERK-Datenbankrolle verwenden."
		return check
	}
	check.Status = diagnostic.Pass
	check.Summary = definition.Key + " hat Schema, Ziel, Datenbank und zweckgebundene Rolle"
	return check
}

func databaseSeparationCheck(definitions []databaseDefinition) diagnostic.Check {
	check := diagnostic.Check{ID: "database.role-separation"}
	roles := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		if !definition.Available {
			check.Status = diagnostic.Fail
			check.Summary = "Trennung der Datenbankrollen kann nicht vollständig geprüft werden"
			check.Diagnosis = "Mindestens eine Prozesskonfiguration ist ungültig."
			check.Remediation = "Alle fünf zweckgebundenen Datenbank-URLs korrigieren."
			return check
		}
		parsed, err := url.Parse(definition.URL)
		if err != nil || parsed.User == nil || parsed.User.Username() == "" {
			check.Status = diagnostic.Fail
			check.Summary = "Trennung der Datenbankrollen kann nicht vollständig geprüft werden"
			check.Diagnosis = "Mindestens eine URL enthält keine auswertbare Rolle."
			check.Remediation = "Jeder Prozessgrenze eine explizite eigene Rolle zuweisen."
			return check
		}
		role := parsed.User.Username()
		if firstKey, duplicate := roles[role]; duplicate {
			check.Status = diagnostic.Fail
			check.Summary = "Datenbankrollen sind nicht nach Prozessgrenze getrennt"
			check.Diagnosis = firstKey + " und " + definition.Key + " verwenden dieselbe Rolle."
			check.Remediation = "Work, Identity, Admin, Worker und Migrator getrennte Rollen geben."
			return check
		}
		roles[role] = definition.Key
	}
	check.Status = diagnostic.Pass
	check.Summary = "Work, Identity, Admin, Worker und Migrator verwenden fünf getrennte Rollen"
	return check
}

func runtimeChecks(
	ctx context.Context,
	prober prober,
	definitions []databaseDefinition,
	kafkaConfiguration config.KafkaConfig,
	baseURL string,
	timeout time.Duration,
) []diagnostic.Check {
	databaseChecks := make([]diagnostic.Check, len(definitions))
	var kafkaCheck diagnostic.Check
	var apiChecks []diagnostic.Check
	var wait sync.WaitGroup
	for index := range definitions {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			databaseChecks[index] = liveDatabaseCheck(ctx, prober, definitions[index], timeout)
		}()
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		kafkaCheck = liveKafkaCheck(ctx, prober, kafkaConfiguration, timeout)
	}()
	if baseURL != "" {
		wait.Add(1)
		go func() {
			defer wait.Done()
			apiChecks = apiinspect.PublicChecks(ctx, prober, baseURL, timeout)
		}()
	}
	wait.Wait()
	checks := append([]diagnostic.Check(nil), databaseChecks...)
	checks = append(checks, kafkaCheck)
	checks = append(checks, apiChecks...)
	return checks
}

func liveDatabaseCheck(ctx context.Context, prober prober, definition databaseDefinition, timeout time.Duration) diagnostic.Check {
	checkContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := prober.CheckDatabase(checkContext, definition.Purpose, definition.URL)
	if err != nil {
		diagnosis := "Verbindung oder serverseitige Rollenprüfung fehlgeschlagen."
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(checkContext.Err(), context.DeadlineExceeded) {
			diagnosis = "Zeitlimit der Datenbankprüfung überschritten."
		} else if errors.Is(err, context.Canceled) || errors.Is(checkContext.Err(), context.Canceled) {
			diagnosis = "Datenbankprüfung wurde abgebrochen."
		}
		return diagnostic.Check{
			ID: "database.connection." + string(definition.Purpose), Status: diagnostic.Fail,
			Summary:     definition.Key + " ist nicht einsatzbereit",
			Diagnosis:   diagnosis,
			Remediation: "Erreichbarkeit, TLS, Credential und die eng begrenzten Rolleneigenschaften prüfen.",
		}
	}
	return diagnostic.Check{
		ID: "database.connection." + string(definition.Purpose), Status: diagnostic.Pass,
		Summary: definition.Key + " verbindet direkt mit der erwarteten Rolle",
	}
}

func liveKafkaCheck(ctx context.Context, prober prober, configuration config.KafkaConfig, timeout time.Duration) diagnostic.Check {
	if !configuration.Enabled {
		return diagnostic.Check{
			ID: "kafka.connection", Status: diagnostic.Pass,
			Summary: "Kafka ist deaktiviert; PostgreSQL-Outbox bleibt autoritativ",
		}
	}
	checkContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := prober.CheckKafka(checkContext, configuration); err != nil {
		diagnosis := "Broker-Verbindung oder Transportprüfung fehlgeschlagen."
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(checkContext.Err(), context.DeadlineExceeded) {
			diagnosis = "Zeitlimit der Kafka-Prüfung überschritten."
		} else if errors.Is(err, context.Canceled) || errors.Is(checkContext.Err(), context.Canceled) {
			diagnosis = "Kafka-Prüfung wurde abgebrochen."
		} else {
			message := strings.ToLower(err.Error())
			switch {
			case strings.Contains(message, "authentication"), strings.Contains(message, "sasl"), strings.Contains(message, "authorization"), strings.Contains(message, "not authorized"):
				diagnosis = "Kafka-Broker erreichbar, aber Authentifizierung oder Topic-Berechtigung wurde abgelehnt."
			case strings.Contains(message, "tls"), strings.Contains(message, "certificate"), strings.Contains(message, "x509"):
				diagnosis = "Kafka-Broker erreichbar, aber TLS-Zertifikat oder Servername wurde abgelehnt."
			case strings.Contains(message, "topic"), strings.Contains(message, "partition"):
				diagnosis = "Broker erreichbar, aber mindestens ein konfiguriertes WERK-Topic fehlt oder ist nicht lesbar."
			}
		}
		return diagnostic.Check{
			ID: "kafka.connection", Status: diagnostic.Fail, Summary: "Aktiviertes Kafka ist nicht erreichbar",
			Diagnosis: diagnosis, Remediation: "Broker, TLS/SASL und Netzwerk prüfen; PostgreSQL-Outbox nicht umgehen.",
		}
	}
	return diagnostic.Check{ID: "kafka.connection", Status: diagnostic.Pass, Summary: "Aktiviertes Kafka antwortet"}
}

func containsFailure(checks []diagnostic.Check) bool {
	for _, check := range checks {
		if check.Status == diagnostic.Fail {
			return true
		}
	}
	return false
}

func validateDirectDevelopmentListener(configuration config.Config) error {
	if configuration.Environment == "production" || configuration.HTTPServerTLS.Enabled() {
		return nil
	}
	host, _, err := net.SplitHostPort(configuration.HTTPAddress)
	if err != nil {
		return err
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	address := net.ParseIP(host)
	if address == nil || !address.IsLoopback() {
		return errors.New("development HTTP without TLS must bind to a loopback address")
	}
	return nil
}

func (prober *runtimeProber) Get(ctx context.Context, endpoint string) (apiinspect.Response, error) {
	return prober.http.Get(ctx, endpoint)
}

func (prober *runtimeProber) CheckDatabase(ctx context.Context, purpose databasePurpose, databaseURL string) error {
	switch purpose {
	case databaseWork:
		connection, err := database.NewWork(ctx, databaseURL, "werkctl-doctor-work")
		if err != nil {
			return err
		}
		connection.Close()
		return nil
	case databaseIdentity:
		connection, err := database.NewIdentity(ctx, databaseURL, "werkctl-doctor-identity")
		if err != nil {
			return err
		}
		connection.Close()
		return nil
	case databaseAdmin:
		connection, err := database.NewAdmin(ctx, databaseURL, "werkctl-doctor-admin")
		if err != nil {
			return err
		}
		connection.Close()
		return nil
	case databaseWorker:
		connection, err := database.NewWorker(ctx, databaseURL, "werkctl-doctor-worker")
		if err != nil {
			return err
		}
		connection.Close()
		return nil
	case databaseMigration:
		connection, err := database.NewMigrationPool(ctx, databaseURL)
		if err != nil {
			return err
		}
		defer connection.Close()
		var sessionUser string
		var currentUser string
		var superuser bool
		var bypassRLS bool
		if err := connection.QueryRow(ctx, `
			SELECT session_user, current_user, role.rolsuper, role.rolbypassrls
			FROM pg_catalog.pg_roles AS role
			WHERE role.rolname = session_user
		`).Scan(&sessionUser, &currentUser, &superuser, &bypassRLS); err != nil {
			return err
		}
		if sessionUser != migrationRole || currentUser != migrationRole || superuser || bypassRLS {
			return errors.New("migration connection uses an unexpected role")
		}
		return nil
	default:
		return errors.New("unsupported database purpose")
	}
}

func (prober *runtimeProber) CheckKafka(ctx context.Context, configuration config.KafkaConfig) error {
	client, err := kafkastream.NewClient(configuration)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.VerifyTopics(ctx)
}
