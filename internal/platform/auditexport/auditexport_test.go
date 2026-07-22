package auditexport

import (
	"testing"
	"time"
)

func TestRetryDelayIsBounded(t *testing.T) {
	if retryDelay(1) != time.Second {
		t.Fatalf("first retry = %s", retryDelay(1))
	}
	if retryDelay(100) != 5*time.Minute {
		t.Fatalf("maximum retry = %s", retryDelay(100))
	}
}
