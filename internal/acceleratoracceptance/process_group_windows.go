//go:build windows

package acceleratoracceptance

import "os/exec"

func setCommandProcessGroup(_ *exec.Cmd) {}

func terminateCommandProcessGroup(command *exec.Cmd) {
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}

func killCommandProcessGroup(command *exec.Cmd) {
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}
