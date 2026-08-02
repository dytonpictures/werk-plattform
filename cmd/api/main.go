package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	webui "github.com/dytonpictures/werk/dashboard"
	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/platform/adminstore"
	"github.com/dytonpictures/werk/internal/platform/businessobjectstore"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/documentstore"
	"github.com/dytonpictures/werk/internal/platform/envfile"
	"github.com/dytonpictures/werk/internal/platform/httpapi"
	identityoidc "github.com/dytonpictures/werk/internal/platform/identityprotocol/oidc"
	"github.com/dytonpictures/werk/internal/platform/identitystore"
	"github.com/dytonpictures/werk/internal/platform/kafkastream"
	"github.com/dytonpictures/werk/internal/platform/providertransport"
	"github.com/dytonpictures/werk/internal/platform/transportsecurity"
	"github.com/dytonpictures/werk/internal/platform/valkeycache"
	"github.com/dytonpictures/werk/internal/platform/workspacestore"
)

func main() {
	if err := envfile.LoadProcess(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid .env file: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.LoadAPI()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	apiStartedAt := time.Now().UTC()
	logger := config.NewLogger(cfg, "api")
	var serverTLSConfiguration *tls.Config
	if cfg.HTTPServerTLS.Enabled() {
		serverTLSConfiguration, err = transportsecurity.NewServerTLSConfig(cfg.HTTPServerTLS)
		if err != nil {
			logger.Error("HTTP server TLS material could not be loaded", "error", err)
			os.Exit(1)
		}
	}
	var kafkaClient *kafkastream.Client
	var logSink *kafkastream.LogSink
	if cfg.Kafka.Enabled {
		kafkaClient, err = kafkastream.NewClient(cfg.Kafka)
		if err != nil {
			logger.Error("Kafka logging client could not be created", "error", err)
			os.Exit(1)
		}
		exporter, exportErr := kafkastream.NewExporter(kafkaClient, cfg.Kafka)
		if exportErr != nil {
			logger.Error("Kafka logging exporter could not be created", "error", exportErr)
			kafkaClient.Close()
			os.Exit(1)
		}
		hostname, _ := os.Hostname()
		instanceID := hostname + "-" + strconv.Itoa(os.Getpid())
		logger, logSink = kafkastream.NewKafkaLogger(logger, exporter, kafkastream.LogMetadata{
			Service: "api", Environment: cfg.Environment,
			BuildVersion: cfg.BuildVersion, InstanceID: instanceID,
		})
	}
	workDatabase, err := database.NewWork(context.Background(), cfg.DatabaseURL, "werk-api-work")
	if err != nil {
		logger.Error("work database could not be created", "error", err)
		os.Exit(1)
	}
	defer workDatabase.Close()
	workspaceService, err := workspacestore.New(workDatabase)
	if err != nil {
		logger.Error("workspace service could not be created", "error", err)
		os.Exit(1)
	}
	documentService, err := documentstore.New(workDatabase)
	if err != nil {
		logger.Error("document service could not be created", "error", err)
		os.Exit(1)
	}
	businessObjectReader, err := businessobjectstore.NewReader(workDatabase)
	if err != nil {
		logger.Error("business object service could not be created", "error", err)
		os.Exit(1)
	}
	businessObjectService := &businessObjectServiceAdapter{reader: businessObjectReader}
	identityDatabase, err := database.NewIdentity(context.Background(), cfg.IdentityDatabaseURL, "werk-api-identity")
	if err != nil {
		logger.Error("identity database could not be created", "error", err)
		os.Exit(1)
	}
	defer identityDatabase.Close()
	identityOptions := []identitystore.Option{
		identitystore.WithMFAKeyring(cfg.IdentityMFAEnabled, cfg.IdentityMFACurrentKeyID, cfg.IdentityMFAKeys),
		identitystore.WithAdminMFARequired(cfg.IdentityAdminMFARequired),
		identitystore.WithSessionLifetimes(cfg.IdentityWorkSessionTTL, cfg.IdentityAdminSessionTTL),
		identitystore.WithSessionIdleLifetimes(cfg.IdentityWorkSessionIdleTTL, cfg.IdentityAdminSessionIdleTTL),
		identitystore.WithWebAuthn(cfg.WebAuthnRPID, cfg.WebAuthnRPName, cfg.WebAuthnOrigins),
	}
	var cacheAdapter *valkeycache.Cache
	if cfg.Cache.Enabled {
		var cacheErr error
		cacheAdapter, cacheErr = valkeycache.New(cfg.Cache.URL)
		if cacheErr != nil {
			logger.Error("optional cache configuration is invalid", "error", cacheErr)
			os.Exit(1)
		} else {
			defer cacheAdapter.Close()
			identityOptions = append(identityOptions, identitystore.WithCache(cacheAdapter))
		}
	}
	authService, err := identitystore.New(
		identityDatabase,
		identityOptions...,
	)
	if err != nil {
		logger.Error("identity service could not be created", "error", err)
		os.Exit(1)
	}
	providerHTTP, err := providertransport.NewClient(providertransport.Config{AllowedSchemes: []string{"https"}, MaxRedirects: 0})
	if err != nil {
		logger.Error("identity provider transport could not be created", "error", err)
		os.Exit(1)
	}
	defer providerHTTP.CloseIdleConnections()
	oidcAdapter, err := identityoidc.NewAdapter(providerHTTP, authService, nil)
	if err != nil {
		logger.Error("OIDC login adapter could not be created", "error", err)
		os.Exit(1)
	}
	oidcLoginService, err := identitystore.NewOIDCLoginCoordinator(authService, oidcAdapter)
	if err != nil {
		logger.Error("OIDC login coordinator could not be created", "error", err)
		os.Exit(1)
	}
	adminDatabase, err := database.NewAdmin(context.Background(), cfg.AdminDatabaseURL, "werk-api-admin")
	if err != nil {
		logger.Error("admin database could not be created", "error", err)
		os.Exit(1)
	}
	defer adminDatabase.Close()
	adminService, err := adminstore.New(
		adminDatabase,
		adminstore.WithRuntimeConfiguration(adminstore.RuntimeConfiguration{
			Environment: cfg.Environment, BuildVersion: cfg.BuildVersion,
			APIVersion: "v1", APIStartedAt: apiStartedAt,
			TransportSecurity: string(cfg.HTTPServerTLS.Mode), KafkaEnabled: cfg.Kafka.Enabled,
			IdentityMFAEnabled: cfg.IdentityMFAEnabled,
			WebAuthnRPID:       cfg.WebAuthnRPID, WebAuthnRPName: cfg.WebAuthnRPName,
			WebAuthnOriginCount: len(cfg.WebAuthnOrigins),
		}),
	)
	if err != nil {
		logger.Error("admin service could not be created", "error", err)
		os.Exit(1)
	}
	if cfg.BootstrapAdminPassword != "" {
		if err := authService.BootstrapAdmin(context.Background(), "admin@werk.local", "Initial Administrator", cfg.BootstrapAdminPassword); err != nil && !errors.Is(err, identity.ErrBootstrapAlreadyUsed) {
			logger.Error("initial administrator could not be bootstrapped", "error", err)
			os.Exit(1)
		}
	}
	if cfg.DevelopmentWorkPassword != "" {
		if err := adminService.EnsureDevelopmentWorkAccount(context.Background(), cfg.DevelopmentWorkPassword); err != nil {
			logger.Error("development work account could not be bootstrapped", "error", err)
			os.Exit(1)
		}
	}

	routerOptions := []httpapi.RouterOption{
		httpapi.WithDocumentService(documentService),
		httpapi.WithBusinessObjectService(businessObjectService),
		httpapi.WithOIDCLoginService(oidcLoginService),
	}
	if logSink != nil {
		routerOptions = append(routerOptions, httpapi.WithRuntimeLogMetrics(logSink.Dropped, logSink.QueuedEntries, logSink.QueuedBytes))
	}
	routerOptions = append(routerOptions, httpapi.WithDatabasePoolMetrics(
		databasePoolMetric("work", workDatabase.PoolSnapshot),
		databasePoolMetric("identity", identityDatabase.PoolSnapshot),
		databasePoolMetric("admin", adminDatabase.PoolSnapshot),
	))
	if cacheAdapter != nil {
		routerOptions = append(routerOptions, httpapi.WithRateLimitCounter(cacheAdapter))
	}
	apiHandler := httpapi.NewRouterWithServices(
		cfg, workDatabase, logger, authService, workspaceService, adminService,
		routerOptions...,
	)
	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           webui.NewHandler(apiHandler),
		TLSConfig:         serverTLSConfiguration,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	go func() {
		logger.Info("api server started", "address", cfg.HTTPAddress, "environment", cfg.Environment, "transport", cfg.HTTPServerTLS.Mode)
		var serveErr error
		if cfg.HTTPServerTLS.Enabled() {
			serveErr = server.ListenAndServeTLS("", "")
		} else {
			serveErr = server.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("api server stopped unexpectedly", "error", serveErr)
			os.Exit(1)
		}
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-signalContext.Done()

	shutdownContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("api server stopped")
	if logSink != nil {
		logCloseContext, logCancel := context.WithTimeout(context.Background(), cfg.Kafka.PublishTimeout+time.Second)
		dropped := logSink.Close(logCloseContext)
		logCancel()
		if dropped > 0 {
			logger.Warn("runtime logs were not exported", "dropped_records", dropped)
		}
	}
	if kafkaClient != nil {
		kafkaClient.Close()
	}
}

func databasePoolMetric(name string, snapshot func() database.PoolSnapshot) httpapi.DatabasePoolMetric {
	return httpapi.DatabasePoolMetric{Name: name, Snapshot: func() httpapi.DatabasePoolSnapshot {
		value := snapshot()
		return httpapi.DatabasePoolSnapshot{
			Max: value.Max, Total: value.Total, Acquired: value.Acquired,
			Idle: value.Idle, Constructing: value.Constructing,
			AcquireWaits: value.AcquireWaits, AcquireTime: value.AcquireTime,
		}
	}}
}

type businessObjectReader interface {
	Resolve(context.Context, identity.AuthenticatedActor, resource.Ref) (businessobjectstore.Resolved, error)
}

// businessObjectServiceAdapter keeps persistence details out of the public HTTP
// contract. In particular, only the Core projection version is exposed; the
// owning module's source version remains internal.
type businessObjectServiceAdapter struct {
	reader businessObjectReader
}

func (adapter *businessObjectServiceAdapter) Resolve(ctx context.Context, actor identity.AuthenticatedActor,
	ref resource.Ref) (httpapi.BusinessObjectRead, bool, error) {
	if adapter == nil || adapter.reader == nil {
		return httpapi.BusinessObjectRead{}, false, errors.New("business object reader is required")
	}
	resolved, err := adapter.reader.Resolve(ctx, actor, ref)
	if errors.Is(err, businessobjectstore.ErrNotFound) {
		return httpapi.BusinessObjectRead{}, false, nil
	}
	if err != nil {
		return httpapi.BusinessObjectRead{}, false, err
	}

	var classification *string
	if resolved.View.Classification != nil {
		value := string(*resolved.View.Classification)
		classification = &value
	}
	return httpapi.BusinessObjectRead{
		Ref:            resolved.View.Ref,
		OwnerModule:    resolved.View.OwnerModule,
		Title:          resolved.View.Title,
		Classification: classification,
		UpdatedAt:      resolved.View.UpdatedAt,
		Version:        resolved.Version,
		Permission:     resolved.ReadPermission,
	}, true, nil
}
