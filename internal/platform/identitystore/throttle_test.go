package identitystore

import "testing"

func TestUnknownLoginThrottleUsesOneBoundedBucket(t *testing.T) {
	first := unknownLoginThrottleKey()
	second := unknownLoginThrottleKey()
	if first != second {
		t.Fatal("unknown login throttle bucket is not stable")
	}
	if first == loginThrottleKey("unknown-account") || first == loginThrottleKey("different-account") {
		t.Fatal("unknown login throttle bucket collides with an account bucket")
	}
}
