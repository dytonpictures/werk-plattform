package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/dytonpictures/werk/internal/platform/auditexport"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/kafkastream"
	"github.com/dytonpictures/werk/internal/platform/outbox"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	logger := config.NewLogger(cfg, "worker")
	hostname, _ := os.Hostname()
	workerID := hostname + "-" + strconv.Itoa(os.Getpid())
	workerDatabase, err := database.NewWorker(context.Background(), cfg.DatabaseURL, "werk-worker")
	if err != nil {
		logger.Error("runtime database could not be created", "error", err)
		os.Exit(1)
	}
	defer workerDatabase.Close()
	store, err := outbox.NewStore(workerDatabase)
	if err != nil {
		logger.Error("outbox store could not be created", "error", err)
		os.Exit(1)
	}
	registry := outbox.NewRegistry()
	var outboxRuntime *outbox.Runtime
	var auditRuntime *auditexport.Runtime
	var kafkaClient *kafkastream.Client
	var logSink *kafkastream.LogSink
	if cfg.Kafka.Enabled {
		kafkaClient, err = kafkastream.NewClient(cfg.Kafka)
		if err != nil {
			logger.Error("Kafka client could not be created", "error", err)
			os.Exit(1)
		}
		checkContext, cancel := context.WithTimeout(context.Background(), cfg.Kafka.PublishTimeout)
		err = kafkaClient.Ping(checkContext)
		cancel()
		if err != nil {
			logger.Error("Kafka is unavailable", "error", err)
			kafkaClient.Close()
			os.Exit(1)
		}
		exporter, exportErr := kafkastream.NewExporter(kafkaClient, cfg.Kafka)
		if exportErr != nil {
			logger.Error("Kafka exporter could not be created", "error", exportErr)
			kafkaClient.Close()
			os.Exit(1)
		}
		domainConsumer, consumerErr := kafkastream.NewDomainConsumer(exporter)
		if consumerErr != nil {
			logger.Error("Kafka domain event consumer could not be registered", "error", consumerErr)
			kafkaClient.Close()
			os.Exit(1)
		}
		if registerErr := registry.Register(domainConsumer); registerErr != nil {
			logger.Error("Kafka domain event consumer could not be registered", "error", registerErr)
			kafkaClient.Close()
			os.Exit(1)
		}
		logger, logSink = kafkastream.NewKafkaLogger(logger, exporter, kafkastream.LogMetadata{
			Service: "worker", Environment: cfg.Environment,
			BuildVersion: cfg.BuildVersion, InstanceID: workerID,
		})
		outboxRuntime, err = outbox.NewRuntime(store, registry, logger, workerID, cfg.WorkerConcurrency)
		if err != nil {
			logger.Error("outbox runtime could not be created", "error", err)
			kafkaClient.Close()
			os.Exit(1)
		}
		auditStore, auditStoreErr := auditexport.NewStore(workerDatabase)
		if auditStoreErr != nil {
			logger.Error("security audit export store could not be created", "error", auditStoreErr)
			kafkaClient.Close()
			os.Exit(1)
		}
		auditRuntime, err = auditexport.NewRuntime(auditStore, exporter, logger, workerID, cfg.Kafka.AuditConcurrency)
		if err != nil {
			logger.Error("security audit export runtime could not be created", "error", err)
			kafkaClient.Close()
			os.Exit(1)
		}
	}

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("worker started", "concurrency", cfg.WorkerConcurrency, "kafka_enabled", cfg.Kafka.Enabled)
	if cfg.Kafka.Enabled {
		var runtimes sync.WaitGroup
		runtimes.Add(2)
		go func() {
			defer runtimes.Done()
			outboxRuntime.Run(signalContext)
		}()
		go func() {
			defer runtimes.Done()
			auditRuntime.Run(signalContext)
		}()
		runtimes.Wait()
	} else {
		logger.Warn("Kafka is disabled; durable events and audit exports remain queued")
		<-signalContext.Done()
	}
	logger.Info("worker stopped")
	if logSink != nil {
		closeContext, cancel := context.WithTimeout(context.Background(), cfg.Kafka.PublishTimeout+time.Second)
		dropped := logSink.Close(closeContext)
		cancel()
		if dropped > 0 {
			logger.Warn("runtime logs were not exported", "dropped_records", dropped)
		}
	}
	if kafkaClient != nil {
		kafkaClient.Close()
	}
}
