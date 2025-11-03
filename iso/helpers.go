package iso

import (
	"fmt"
	"os/exec"
)

func run(name string, args ...string) (output string, err error) {
	var (
		cmd    *exec.Cmd
		stdout []byte
	)

	cmd = exec.Command(name, args...)

	if stdout, err = cmd.Output(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			err = fmt.Errorf("failed to run command: %s", string(exitErr.Stderr))
			return
		}

		err = fmt.Errorf("failed to run command: %s", err)
		return
	}

	output = string(stdout)
	return
}

func cleanMount(point string) (err error) {
	if _, err = run("umount", point); err != nil {
		err = fmt.Errorf("failed to unmount %s: %s", point, err)
		return
	}

	if _, err = run("rm", "-rf", point); err != nil {
		err = fmt.Errorf("failed to remove %s: %s", point, err)
		return
	}

	return nil
}
