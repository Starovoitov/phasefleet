// Package clienv reads process environment values for command flags.
// A blank variable is treated as unset, except Optional, which keeps a
// blank so a caller can disable a default.
package clienv

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// String returns the variable, or fallback when it is unset or blank.
func String(key, fallback string) string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// Optional returns the variable when it is set, including a blank value.
// Unset returns fallback.
func Optional(key, fallback string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return v
}

// Float parses the variable. ok is false when it is unset or blank.
func Float(key string) (value float64, ok bool, err error) {
	v, found := os.LookupEnv(key)
	if !found || strings.TrimSpace(v) == "" {
		return 0, false, nil
	}
	value, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, true, fmt.Errorf("env %s: %w", key, err)
	}
	return value, true, nil
}

// Int parses the variable. ok is false when it is unset or blank.
func Int(key string) (value int, ok bool, err error) {
	v, found := os.LookupEnv(key)
	if !found || strings.TrimSpace(v) == "" {
		return 0, false, nil
	}
	value, err = strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, true, fmt.Errorf("env %s: %w", key, err)
	}
	return value, true, nil
}
