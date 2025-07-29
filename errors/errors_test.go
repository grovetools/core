package errors

import (
	"fmt"
	"testing"
)

func TestGroveError(t *testing.T) {
	// Test basic error creation
	err := New(ErrCodeServiceNotFound, "service not found")
	if err.Code != ErrCodeServiceNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeServiceNotFound, err.Code)
	}

	// Test error wrapping
	cause := fmt.Errorf("underlying error")
	wrapped := Wrap(cause, ErrCodeCommandFailed, "command failed")

	if wrapped.Unwrap() != cause {
		t.Error("Unwrap should return the cause")
	}

	// Test Is function
	if !Is(wrapped, ErrCodeCommandFailed) {
		t.Error("Is should return true for matching code")
	}

	if Is(wrapped, ErrCodeServiceNotFound) {
		t.Error("Is should return false for non-matching code")
	}

	// Test WithDetail
	detailed := err.WithDetail("service", "web").WithDetail("port", 8080)
	if detailed.Details["service"] != "web" {
		t.Error("WithDetail should add details")
	}
}

func TestErrorConstructors(t *testing.T) {
	// Test ServiceNotFound
	err := ServiceNotFound("web")
	if err.Code != ErrCodeServiceNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeServiceNotFound, err.Code)
	}
	if err.Details["service"] != "web" {
		t.Error("ServiceNotFound should include service detail")
	}

	// Test PortConflict
	err = PortConflict(8080, "nginx")
	if err.Code != ErrCodePortConflict {
		t.Errorf("expected code %s, got %s", ErrCodePortConflict, err.Code)
	}
	if err.Details["port"] != 8080 {
		t.Error("PortConflict should include port detail")
	}
}