package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TBG-Chance/SentinelBox/internal/config"
)

func TestNewProducesStructuredJSON(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(config.LoggingConfig{Level: "info", Format: "json"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("ready", "component", "test")
	text := output.String()
	for _, expected := range []string{`"msg":"ready"`, `"service":"sentinelbox"`, `"component":"test"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("log output %q does not contain %q", text, expected)
		}
	}
}

func TestNewRejectsUnsupportedSettings(t *testing.T) {
	var output bytes.Buffer
	if _, err := New(config.LoggingConfig{Level: "verbose", Format: "json"}, &output); err == nil {
		t.Fatal("New accepted an unsupported level")
	}
	if _, err := New(config.LoggingConfig{Level: "info", Format: "xml"}, &output); err == nil {
		t.Fatal("New accepted an unsupported format")
	}
}
