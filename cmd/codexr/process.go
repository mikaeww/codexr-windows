package main

import (
	"os"
	"os/exec"
)

func runProcess(executable string, args, environment []string) error {
	command := exec.Command(executable, args...)
	command.Env = environment
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}
