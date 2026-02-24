//go:build tools

// Package tools tracks development tool dependencies.
//
// This file ensures that `go mod tidy` doesn't remove tool dependencies
// from go.mod. Tools can be installed with `go install` commands or via
// `just install-tools`.
package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "golang.org/x/tools/cmd/goimports"
)
