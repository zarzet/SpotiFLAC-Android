package gobackend

import (
	"fmt"
	"os"
	"strings"
)

func isFDOutput(outputFD int) bool {
	return outputFD > 0
}

func openOutputForWrite(outputPath string, outputFD int) (*os.File, error) {
	if isFDOutput(outputFD) {
		return os.NewFile(uintptr(outputFD), fmt.Sprintf("saf_fd_%d", outputFD)), nil
	}

	path := strings.TrimSpace(outputPath)
	if strings.HasPrefix(path, "/proc/self/fd/") {
		// Re-open procfs fd path instead of taking ownership of raw detached fd.
		// Some SAF providers reject O_TRUNC on these descriptors with EACCES/EPERM.
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
		if err == nil {
			return file, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "permission denied") {
			return os.OpenFile(path, os.O_WRONLY, 0)
		}
		return nil, err
	}

	return os.Create(outputPath)
}

func cleanupOutputOnError(outputPath string, outputFD int) {
	if isFDOutput(outputFD) {
		return
	}

	path := strings.TrimSpace(outputPath)
	if path == "" || strings.HasPrefix(path, "/proc/self/fd/") {
		return
	}

	_ = os.Remove(path)
}
