package outbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/events"
	"github.com/dytonpictures/werk/internal/platform/database"
)

func TestRetryDelayIsBounded(t *testing.T) {
	if retryDelay(1) != time.Second {
		t.Fatalf("first retry = %s", retryDelay(1))
	}
	if retryDelay(100) != 5*time.Minute {
		t.Fatalf("maximum retry = %s", retryDelay(100))
	}
}

type testConsumer struct {
	key       string
	eventType string
}

func (consumer testConsumer) Key() string       { return consumer.key }
func (consumer testConsumer) EventType() string { return consumer.eventType }
func (consumer testConsumer) Handle(context.Context, database.TenantTx, events.Event) error {
	return nil
}

func TestRegistryAddsAllEventConsumer(t *testing.T) {
	registry := NewRegistry()
	global := testConsumer{key: "platform.kafka.domain.v1", eventType: AllEventTypes}
	specific := testConsumer{key: "core.example.consumer.v1", eventType: "core.example.created.v1"}
	if err := registry.Register(global); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(specific); err != nil {
		t.Fatal(err)
	}
	consumers := registry.Consumers("core.example.created.v1")
	if len(consumers) != 2 || consumers[0].Key() != specific.Key() || consumers[1].Key() != global.Key() {
		t.Fatalf("unexpected consumers: %#v", consumers)
	}
	if len(registry.Consumers("core.other.created.v1")) != 1 {
		t.Fatal("global event consumer did not match another event type")
	}
}

func TestEventTagJSONRemainsAnObject(t *testing.T) {
	encoded, err := json.Marshal(events.NormalizeTags(nil))
	if err != nil || len(encoded) == 0 || encoded[0] != '{' {
		t.Fatalf("invalid normalized tags: %s, %v", encoded, err)
	}
}
