package desktop

import "errors"

type Instance struct {
	PID, AppPID, LauncherPID int
	CodexHome, Port          string
}

var unsupported = errors.New("desktop switching requires the Linux desktop launcher; on Windows use codexr run or press s to launch the Codex CLI")

func Launcher() (string, error)          { return "", unsupported }
func Running() []Instance                { return nil }
func RunningFor(string) (Instance, bool) { return Instance{}, false }
func Launch(string, bool) error          { return unsupported }
func Restart(string) error               { return unsupported }
func Stop(Instance) error                { return unsupported }
