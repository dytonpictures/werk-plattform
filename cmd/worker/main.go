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

	"github.com/dytonpictures/werk/internal/core/operations"
	"github.com/dytonpictures/werk/internal/platform/auditexport"
	"github.com/dytonpictures/werk/internal/platform/config"
	"github.com/dytonpictures/werk/internal/platform/database"
	"github.com/dytonpictures/werk/internal/platform/envfile"
	"github.com/dytonpictures/werk/internal/platform/kafkastream"
	"github.com/dytonpictures/werk/internal/platform/operationsstore"
	"github.com/dytonpictures/werk/internal/platform/outbox"
)

const (
	workerHeartbeatInterval       = 10 * time.Second
	workerHeartbeatExpiry         = 30 * time.Second
	workerHeartbeatAttemptTimeout = 5 * time.Second
	kafkaObservationInterval      = 30 * time.Second
)

type heartbeatUpdater interface {
	Beat(context.Context, operations.State) error
}

func main() {
	if err := envfile.LoadProcess(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid .env file: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.LoadWorker()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}
	logger := config.NewComponentLogger(cfg.Environment, cfg.BuildVersion, "worker")
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
	asyncWakeups := make(chan struct{}, 1)
	var outboxRuntime *outbox.Runtime
	var auditRuntime *auditexport.Runtime
	var kafkaClient *kafkastream.Client
	var kafkaObservedAt time.Time
	var kafkaObservedFailures uint64
	var logSink *kafkastream.LogSink
	if cfg.Kafka.Enabled {
		kafkaClient, err = kafkastream.NewClient(cfg.Kafka)
		if err != nil {
			logger.Error("Kafka client could not be created", "error", err)
			os.Exit(1)
		}
		checkContext, cancel := context.WithTimeout(context.Background(), cfg.Kafka.PublishTimeout)
		err = kafkaClient.VerifyTopics(checkContext)
		cancel()
		if err != nil {
			logger.Error("Kafka topic contract is unavailable", "error", err)
			kafkaClient.Close()
			os.Exit(1)
		}
		kafkaObservedAt = time.Now()
		kafkaObservedFailures = kafkaClient.PublishFailures()
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
		outboxRuntime, err = outbox.NewRuntime(store, registry, logger, workerID, cfg.WorkerConcurrency, asyncWakeups)
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
		auditRuntime, err = auditexport.NewRuntime(auditStore, exporter, logger, workerID, cfg.Kafka.AuditConcurrency, asyncWakeups)
		if err != nil {
			logger.Error("security audit export runtime could not be created", "error", err)
			kafkaClient.Close()
			os.Exit(1)
		}
	}
	heartbeatWriter, err := operationsstore.NewWorkerHeartbeatWriter(workerDatabase, cfg.BuildVersion, workerHeartbeatExpiry)
	if err != nil {
		logger.Error("worker heartbeat could not be configured", "error", err)
		os.Exit(1)
	}
	initialHeartbeatContext, cancelInitialHeartbeat := context.WithTimeout(context.Background(), workerHeartbeatAttemptTimeout)
	initialKafkaState := operations.StateDisabled
	if cfg.Kafka.Enabled {
		initialKafkaState = operations.StateReady
	}
	err = heartbeatWriter.Beat(initialHeartbeatContext, initialKafkaState)
	cancelInitialHeartbeat()
	if err != nil {
		logger.Error("initial worker heartbeat failed", "error", err)
		os.Exit(1)
	}

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var listenerRuntime sync.WaitGroup
	if cfg.Kafka.Enabled {
		listenerRuntime.Add(1)
		go func() {
			defer listenerRuntime.Done()
			for signalContext.Err() == nil {
				if err := workerDatabase.ListenAsyncWork(signalContext, asyncWakeups); err != nil && signalContext.Err() == nil {
					logger.WarnContext(signalContext, "async work listener failed; polling remains active", "error", err)
				}
				select {
				case <-signalContext.Done():
					return
				case <-time.After(time.Second):
				}
			}
		}()
	}
	var heartbeatRuntime sync.WaitGroup
	heartbeatRuntime.Add(1)
	go func() {
		defer heartbeatRuntime.Done()
		ticker := time.NewTicker(workerHeartbeatInterval)
		defer ticker.Stop()
		runWorkerHeartbeat(signalContext, heartbeatWriter, ticker.C, workerHeartbeatAttemptTimeout, func(ctx context.Context) (operations.State, error) {
			if !cfg.Kafka.Enabled {
				return operations.StateDisabled, nil
			}
			publishFailures := kafkaClient.PublishFailures()
			if publishFailures == kafkaObservedFailures && time.Since(kafkaObservedAt) < kafkaObservationInterval {
				return operations.StateReady, nil
			}
			if err := kafkaClient.VerifyTopics(ctx); err != nil {
				return operations.StateDegraded, err
			}
			kafkaObservedAt = time.Now()
			kafkaObservedFailures = publishFailures
			return operations.StateReady, nil
		}, func(err error) {
			logger.WarnContext(signalContext, "worker heartbeat update failed", "error", err)
		})
	}()
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
	heartbeatRuntime.Wait()
	listenerRuntime.Wait()
	removeHeartbeatContext, cancelRemoveHeartbeat := context.WithTimeout(context.Background(), workerHeartbeatAttemptTimeout)
	if err := heartbeatWriter.Remove(removeHeartbeatContext); err != nil {
		logger.Warn("worker heartbeat could not be withdrawn", "error", err)
	}
	cancelRemoveHeartbeat()
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

// runWorkerHeartbeat deliberately performs no initial update. The caller must
// complete that update synchronously before announcing the worker as started.
// Later failures are reported and retried on the next tick until shutdown.
func runWorkerHeartbeat(
	ctx context.Context,
	writer heartbeatUpdater,
	ticks <-chan time.Time,
	attemptTimeout time.Duration,
	observeKafka func(context.Context) (operations.State, error),
	reportFailure func(error),
) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			kafkaState := operations.StateUnknown
			var observationErr error
			if observeKafka != nil {
				observationContext, cancelObservation := context.WithTimeout(ctx, attemptTimeout)
				kafkaState, observationErr = observeKafka(observationContext)
				cancelObservation()
			}
			if observationErr != nil && ctx.Err() == nil && reportFailure != nil {
				reportFailure(fmt.Errorf("Kafka observation failed: %w", observationErr))
			}
			heartbeatContext, cancelHeartbeat := context.WithTimeout(ctx, attemptTimeout)
			err := writer.Beat(heartbeatContext, kafkaState)
			cancelHeartbeat()
			if err != nil && ctx.Err() == nil && reportFailure != nil {
				reportFailure(err)
			}
		}
	}
}
