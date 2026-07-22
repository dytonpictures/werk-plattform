package kafkastream

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dytonpictures/werk/internal/core/events"
)

const (
	logQueueCapacity    = 2_048
	maximumLogBodyBytes = 256 << 10
)

type LogMetadata struct {
	Service      string
	Environment  string
	BuildVersion string
	InstanceID   string
}

type logEnvelope struct {
	SpecVersion  string            `json:"spec_version"`
	Category     string            `json:"category"`
	ID           string            `json:"id"`
	OccurredAt   time.Time         `json:"occurred_at"`
	Level        string            `json:"level"`
	Message      string            `json:"message"`
	Service      string            `json:"service"`
	Environment  string            `json:"environment"`
	BuildVersion string            `json:"build_version"`
	InstanceID   string            `json:"instance_id"`
	Tags         map[string]string `json:"tags"`
	Attributes   map[string]any    `json:"attributes,omitempty"`
}

type queuedLog struct {
	message Message
}

type LogSink struct {
	writer   Writer
	topic    string
	queue    chan queuedLog
	stop     chan struct{}
	done     chan struct{}
	closed   atomic.Bool
	dropped  atomic.Uint64
	stopOnce sync.Once
}

func NewKafkaLogger(base *slog.Logger, exporter *Exporter, metadata LogMetadata) (*slog.Logger, *LogSink) {
	if base == nil || exporter == nil || exporter.writer == nil {
		return base, nil
	}
	sink := &LogSink{
		writer: exporter.writer, topic: exporter.logsTopic,
		queue: make(chan queuedLog, logQueueCapacity),
		stop:  make(chan struct{}), done: make(chan struct{}),
	}
	go sink.run()
	handler := &kafkaLogHandler{base: base.Handler(), sink: sink, metadata: metadata}
	return slog.New(handler), sink
}

func (sink *LogSink) run() {
	defer close(sink.done)
	for {
		select {
		case record := <-sink.queue:
			if err := sink.writer.Publish(context.Background(), record.message); err != nil {
				sink.dropped.Add(1)
			}
		case <-sink.stop:
			return
		}
	}
}

func (sink *LogSink) Close(ctx context.Context) uint64 {
	if sink == nil {
		return 0
	}
	sink.closed.Store(true)
	sink.stopOnce.Do(func() { close(sink.stop) })
	select {
	case <-sink.done:
	case <-ctx.Done():
	}
	return sink.dropped.Load()
}

type kafkaLogHandler struct {
	base     slog.Handler
	sink     *LogSink
	metadata LogMetadata
	attrs    []slog.Attr
	groups   []string
}

func (handler *kafkaLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.base.Enabled(ctx, level)
}

func (handler *kafkaLogHandler) Handle(ctx context.Context, record slog.Record) error {
	if err := handler.base.Handle(ctx, record); err != nil {
		return err
	}
	if handler.sink.closed.Load() {
		return nil
	}
	attributes := make(map[string]any, len(handler.attrs)+record.NumAttrs())
	for _, attribute := range handler.attrs {
		appendLogAttribute(attributes, handler.groups, attribute)
	}
	record.Attrs(func(attribute slog.Attr) bool {
		appendLogAttribute(attributes, handler.groups, attribute)
		return true
	})
	identifier, err := randomUUID()
	if err != nil {
		handler.sink.dropped.Add(1)
		return nil
	}
	tags := map[string]string{
		events.TagDataClassification: "confidential",
		events.TagProcessingPurpose:  "platform-operations",
		events.TagRetentionClass:     "operational-log",
	}
	envelope := logEnvelope{
		SpecVersion: envelopeVersion, Category: "runtime-log", ID: identifier,
		OccurredAt: record.Time.UTC(), Level: record.Level.String(), Message: record.Message,
		Service: handler.metadata.Service, Environment: handler.metadata.Environment,
		BuildVersion: handler.metadata.BuildVersion, InstanceID: handler.metadata.InstanceID,
		Tags: tags, Attributes: attributes,
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > maximumLogBodyBytes {
		handler.sink.dropped.Add(1)
		return nil
	}
	key := handler.metadata.InstanceID
	if correlationID, ok := attributes["correlation_id"].(string); ok && correlationID != "" {
		key = correlationID
	}
	message := Message{
		Topic: handler.sink.topic, Key: key, Value: encoded, Timestamp: record.Time.UTC(),
		Headers: map[string]string{
			"content-type": "application/json", "spec-version": envelopeVersion,
			"event-id": identifier, "event-type": "platform.runtime-log.v1",
			"data-classification": tags[events.TagDataClassification],
		},
	}
	select {
	case handler.sink.queue <- queuedLog{message: message}:
	default:
		handler.sink.dropped.Add(1)
	}
	return nil
}

func (handler *kafkaLogHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	clone := *handler
	clone.base = handler.base.WithAttrs(attributes)
	clone.attrs = append(append([]slog.Attr(nil), handler.attrs...), attributes...)
	return &clone
}

func (handler *kafkaLogHandler) WithGroup(name string) slog.Handler {
	clone := *handler
	clone.base = handler.base.WithGroup(name)
	clone.groups = append(append([]string(nil), handler.groups...), name)
	return &clone
}

func appendLogAttribute(target map[string]any, groups []string, attribute slog.Attr) {
	attribute.Value = attribute.Value.Resolve()
	key := attribute.Key
	if len(groups) > 0 {
		key = strings.Join(append(append([]string(nil), groups...), key), ".")
	}
	if sensitiveLogKey(key) {
		target[key] = "[REDACTED]"
		return
	}
	target[key] = logValue(attribute.Value)
}

func logValue(value slog.Value) any {
	switch value.Kind() {
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindString:
		return boundedString(value.String(), 32<<10)
	case slog.KindTime:
		return value.Time().UTC()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindGroup:
		group := make(map[string]any)
		for _, attribute := range value.Group() {
			appendLogAttribute(group, nil, attribute)
		}
		return group
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return boundedString(err.Error(), 32<<10)
		}
		return boundedString(fmt.Sprint(value.Any()), 32<<10)
	default:
		return nil
	}
}

func sensitiveLogKey(key string) bool {
	lower := strings.ToLower(key)
	for _, marker := range []string{"password", "token", "secret", "authorization", "cookie", "credential", "session"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func boundedString(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}

func randomUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return uuidString(value), nil
}
