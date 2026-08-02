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
	// Runtime logs are explicitly loss-tolerant. At the maximum encoded record
	// size the byte budget is authoritative, while the entry limit preserves
	// capacity for ordinary small log bursts.
	logQueueCapacity    = 2_048
	logQueueByteBudget  = 64 << 20
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
	bytes   int64
}

type LogSink struct {
	writer   Writer
	topic    string
	queue    chan queuedLog
	stop     chan context.Context
	done     chan struct{}
	runCtx   context.Context
	cancel   context.CancelFunc
	stateMu  sync.RWMutex
	closed   bool
	dropped  atomic.Uint64
	queued   atomic.Int64
	stopOnce sync.Once
}

func NewKafkaLogger(base *slog.Logger, exporter *Exporter, metadata LogMetadata) (*slog.Logger, *LogSink) {
	if base == nil || exporter == nil || exporter.writer == nil {
		return base, nil
	}
	runContext, cancel := context.WithCancel(context.Background())
	sink := &LogSink{
		writer: exporter.writer, topic: exporter.logsTopic,
		queue: make(chan queuedLog, logQueueCapacity),
		stop:  make(chan context.Context, 1), done: make(chan struct{}),
		runCtx: runContext, cancel: cancel,
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
			sink.publish(sink.runCtx, record)
		case closeContext := <-sink.stop:
			sink.drain(closeContext)
			return
		}
	}
}

func (sink *LogSink) publish(ctx context.Context, record queuedLog) {
	defer sink.queued.Add(-record.bytes)
	if err := sink.writer.Publish(ctx, record.message); err != nil {
		sink.dropped.Add(1)
	}
}

func (sink *LogSink) drain(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			sink.discardQueued()
			return
		case record := <-sink.queue:
			sink.publish(ctx, record)
		default:
			return
		}
	}
}

func (sink *LogSink) discardQueued() {
	for {
		select {
		case record := <-sink.queue:
			sink.queued.Add(-record.bytes)
			sink.dropped.Add(1)
		default:
			return
		}
	}
}

func (sink *LogSink) Close(ctx context.Context) uint64 {
	if sink == nil {
		return 0
	}
	sink.stopOnce.Do(func() {
		sink.stateMu.Lock()
		sink.closed = true
		sink.stop <- ctx
		sink.stateMu.Unlock()
	})
	select {
	case <-sink.done:
	case <-ctx.Done():
		sink.cancel()
		<-sink.done
	}
	sink.cancel()
	return sink.dropped.Load()
}

// Dropped returns the number of runtime-log records that could not be
// exported because encoding, buffering, publishing or bounded shutdown failed.
func (sink *LogSink) Dropped() uint64 {
	if sink == nil {
		return 0
	}
	return sink.dropped.Load()
}

// QueuedEntries and QueuedBytes expose bounded, low-cardinality process state
// for metrics. Bytes include a record while it is being synchronously
// published, so the reported budget cannot appear free prematurely.
func (sink *LogSink) QueuedEntries() int {
	if sink == nil {
		return 0
	}
	return len(sink.queue)
}

func (sink *LogSink) QueuedBytes() int64 {
	if sink == nil {
		return 0
	}
	return sink.queued.Load()
}

type kafkaLogHandler struct {
	base     slog.Handler
	sink     *LogSink
	metadata LogMetadata
	attrs    []slog.Attr
	group    string
}

func (handler *kafkaLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.base.Enabled(ctx, level)
}

func (handler *kafkaLogHandler) Handle(ctx context.Context, record slog.Record) error {
	if err := handler.base.Handle(ctx, record); err != nil {
		return err
	}
	attributeCount := len(handler.attrs) + record.NumAttrs()
	var attributes map[string]any
	if attributeCount > 0 {
		attributes = make(map[string]any, attributeCount)
	}
	for _, attribute := range handler.attrs {
		appendLogAttribute(attributes, handler.group, attribute)
	}
	record.Attrs(func(attribute slog.Attr) bool {
		appendLogAttribute(attributes, handler.group, attribute)
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
	messageBytes := int64(len(encoded))
	handler.sink.stateMu.RLock()
	if handler.sink.closed {
		handler.sink.stateMu.RUnlock()
		return nil
	}
	if !handler.sink.reserve(messageBytes) {
		handler.sink.dropped.Add(1)
		handler.sink.stateMu.RUnlock()
		return nil
	}
	select {
	case handler.sink.queue <- queuedLog{message: message, bytes: messageBytes}:
	default:
		handler.sink.queued.Add(-messageBytes)
		handler.sink.dropped.Add(1)
	}
	handler.sink.stateMu.RUnlock()
	return nil
}

func (sink *LogSink) reserve(size int64) bool {
	if size <= 0 || size > logQueueByteBudget {
		return false
	}
	for {
		current := sink.queued.Load()
		if current > logQueueByteBudget-size {
			return false
		}
		if sink.queued.CompareAndSwap(current, current+size) {
			return true
		}
	}
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
	if clone.group == "" {
		clone.group = name
	} else {
		clone.group += "." + name
	}
	return &clone
}

func appendLogAttribute(target map[string]any, group string, attribute slog.Attr) {
	attribute.Value = attribute.Value.Resolve()
	key := attribute.Key
	if group != "" {
		key = group + "." + key
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
			appendLogAttribute(group, "", attribute)
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
	segments := strings.Split(lower, ".")
	if len(segments) > 0 && segments[len(segments)-1] == "error" {
		return true
	}
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
