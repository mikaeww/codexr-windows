//go:build !windows

package state

import "os"

func secureDirectory(path string) error { return os.Chmod(path, 0o700) }
