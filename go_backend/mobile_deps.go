// mobile_deps.go
// This file ensures gomobile dependencies are not removed by go mod tidy.
// These packages are required by gomobile bind but not directly imported in code.

package gobackend

import (
	// Required for gomobile bind to work
	_ "golang.org/x/mobile/bind"
)
