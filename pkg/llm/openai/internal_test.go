// Package openai internal white-box tests.
// These live in package openai (not openai_test) so they can access unexported
// seams like jsonMarshal. Only add tests here when a black-box test is impossible.
package openai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestChat_MarshalError(t *testing.T) {
	// Replace the jsonMarshal seam to simulate a json encoding failure.
	orig := jsonMarshal
	jsonMarshal = func(v any) ([]byte, error) {
		return nil, errors.New("simulated marshal failure")
	}
	defer func() { jsonMarshal = orig }()

	client := New(Config{BaseURL: "http://irrelevant", APIKey: "k", Model: "m"})
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error from failing marshal")
	}
	if !strings.Contains(err.Error(), "marshal request") {
		t.Errorf("error %q should mention 'marshal request'", err.Error())
	}
}
