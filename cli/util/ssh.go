// Copyright 2025 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package util

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type SSHCommand struct {
	Cmd     *exec.Cmd
	ctx     context.Context
	Stdout  io.ReadCloser
	cancel  context.CancelFunc
	sigChan chan os.Signal
	errChan chan error
}

func StartSSHCommand(c *cli.Context,
	_ctx context.Context,
	cancel context.CancelFunc,
	command string,
	sshConnectedToken string,
) (*SSHCommand, error) {

	var userAtHost string
	var sigChan chan os.Signal
	var errChan chan error
	port := "22"
	host := strings.TrimPrefix(c.String("file"), "ssh://")

	if remotePort := strings.Split(host, ":"); len(remotePort) == 2 {
		port = remotePort[1]
		userAtHost = remotePort[0]
	} else {
		userAtHost = host
	}

	args := c.StringSlice("ssh-args")
	// Check if port is specified explicitly with the --ssh-args flag
	addPort := true
	for _, arg := range args {
		if strings.Contains(arg, "-p") {
			addPort = false
			break
		}
	}
	if addPort {
		args = append(args, "-p", port)
	}
	args = append(args, userAtHost)
	args = append(
		args,
		"/bin/sh",
		"-c",
		command)

	cmd := exec.Command("ssh", args...)

	// Simply connect stdin/stderr
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.New("Error redirecting stdout on exec")
	}

	// Disable tty echo before starting
	term, err := DisableEcho(int(os.Stdin.Fd()))
	if err == nil {
		sigChan = make(chan os.Signal, 1)
		errChan = make(chan error, 1)
		// Make sure that echo is enabled if the process gets
		// interrupted
		signal.Notify(sigChan)
		go EchoSigHandler(_ctx, sigChan, errChan, term)
	} else if err != syscall.ENOTTY {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Wait for 120 seconds for ssh to establish connection
	err = waitForBufferSignal(stdout, os.Stdout, sshConnectedToken, 2*time.Minute)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, errors.Wrap(err,
			"Error waiting for ssh session to be established.")
	}
	return &SSHCommand{
		ctx:     _ctx,
		Cmd:     cmd,
		Stdout:  stdout,
		cancel:  cancel,
		sigChan: sigChan,
		errChan: errChan,
	}, nil
}

func (s *SSHCommand) EndSSHCommand() error {
	if s.Cmd.ProcessState != nil && s.Cmd.ProcessState.Exited() {
		return errors.New("SSH session closed unexpectedly")
	}

	if err := s.Cmd.Wait(); err != nil {
		return errors.Wrap(err,
			"SSH session closed with error")
	}

	if s.sigChan != nil {
		signal.Stop(s.sigChan)
		s.cancel()
		if err := <-s.errChan; err != nil {
			return err
		}
	} else {
		s.cancel()
	}

	return nil
}

// Reads from src waiting for the string specified by signal, writing all other
// output appearing at src to sink. The function returns an error if occurs
// reading from the stream or the deadline exceeds.
func waitForBufferSignal(src io.Reader, sink io.Writer,
	signal string, deadline time.Duration) error {

	var err error
	errChan := make(chan error)

	go func() {
		stdoutRdr := bufio.NewReader(src)
		for {
			line, err := stdoutRdr.ReadString('\n')
			if err != nil {
				errChan <- err
				break
			}
			if strings.Contains(line, signal) {
				errChan <- nil
				break
			}
			_, err = sink.Write([]byte(line + "\n"))
			if err != nil {
				errChan <- err
				break
			}
		}
	}()

	select {
	case err = <-errChan:
		// Error from goroutine
	case <-time.After(deadline):
		err = errors.New("Input deadline exceeded")
	}
	return err
}
