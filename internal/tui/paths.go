package tui

import "os"

func homeDirectory() (string, error) {
	return os.UserHomeDir()
}
