package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func CMDRun(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("exec %s: %w", command, err)
	}
	return nil
}

func SSHTest() error {
	env, err := GetENV()
	if err != nil {
		return err
	}

	cmdArgs := []string{
		"-p", env.Password,
		"ssh",
		"-o", "ConnectTimeout=3",
		"-o", "StrictHostKeyChecking=no",
		"-q", env.Remote,
		"exit",
	}
	if _, err := exec.Command("sshpass", cmdArgs...).Output(); err != nil {
		return err
	}
	return nil
}

func SSHRun(args ...string) error {
	env, err := GetENV()
	if err != nil {
		return err
	}

	command := strings.Join(args, " ")
	cmdArgs := []string{
		"-p", env.Password,
		"ssh",
		"-tt",
		"-o", "StrictHostKeyChecking=no",
		"-o", "LogLevel=QUIET",
		env.Remote,
		command,
	}
	cmd := exec.Command("sshpass", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func SSEOutput(args ...string) (string, error) {
	env, err := GetENV()
	if err != nil {
		return "", err
	}

	command := strings.Join(args, " ")
	cmdArgs := []string{
		"-p", env.Password,
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "LogLevel=QUIET",
		env.Remote,
		command,
	}
	out, err := exec.Command("sshpass", cmdArgs...).Output()
	return string(out), err
}
