package e2e

import (
	"errors"
	"testing"
	"time"
)

func TestRetryUntilSuccessEventuallySucceeds(t *testing.T) {
	attempts := 0

	err := retryUntilSuccess(200*time.Millisecond, 5*time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected retryUntilSuccess to eventually succeed, got error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryUntilSuccessReturnsLastError(t *testing.T) {
	expectedErr := errors.New("still failing")

	err := retryUntilSuccess(25*time.Millisecond, 5*time.Millisecond, func() error {
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected last error %v, got %v", expectedErr, err)
	}
}
