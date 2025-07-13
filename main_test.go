package main

import (
	"testing"
)

func TestVersion(t *testing.T) {
	if version == "" {
		t.Error("Version should not be empty")
	}
}

func TestAppName(t *testing.T) {
	expected := "CloudAWSync"
	if appName != expected {
		t.Errorf("Expected app name %s, got %s", expected, appName)
	}
}
