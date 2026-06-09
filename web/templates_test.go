package web

import "testing"

func TestNewRendererParsesTemplates(t *testing.T) {
	if _, err := NewRenderer(); err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
}
