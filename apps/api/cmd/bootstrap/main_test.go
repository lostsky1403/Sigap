package main

import (
	"os"
	"testing"
)

func TestCheckEnabled(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{"explicitly true", "true", true},
		{"explicitly false", "false", false},
		{"empty string", "", false},
		{"unset", "", false},
		{"random value", "yes", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" || tt.name == "empty string" {
				t.Setenv("SIGAP_BOOTSTRAP_ADMIN", tt.env)
			}
			if tt.name == "unset" {
				os.Unsetenv("SIGAP_BOOTSTRAP_ADMIN")
			}
			if got := checkEnabled(); got != tt.want {
				t.Errorf("checkEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
