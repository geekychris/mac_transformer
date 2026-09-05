package core

import "os/exec"

// runCmd is a tiny test helper — spawns a command and returns its combined
// output as a string.
func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
