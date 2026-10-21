package main

import (
	"fmt"
	"strings"
)

func streamAddress(source string) (string, error) {
	if !strings.HasPrefix(source, "stream://") {
		return "", fmt.Errorf("invalid actionstream-source %q: must be stream://host:port or stream:///path/to.sock", source)
	}
	addr := strings.TrimPrefix(source, "stream://")
	if addr == "" {
		return "", fmt.Errorf("actionstream-source is required")
	}
	return addr, nil
}
