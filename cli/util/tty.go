// Copyright 2021 Northern.tech AS
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

//go:build linux || aix || darwin || dragonfly || freebsd || netbsd || opbenbsd
// +build linux aix darwin dragonfly freebsd netbsd opbenbsd

package util

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// Disable TTY echo of stdin.
func DisableEcho(fd int) (*unix.Termios, error) {
	term, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return nil, err
	}

	newTerm := *term
	newTerm.Lflag &^= unix.ECHO
	newTerm.Lflag |= unix.ICANON | unix.ISIG
	newTerm.Iflag |= unix.ICRNL
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &newTerm); err != nil {
		return nil, err
	}
	return term, nil
}

// Signal handler to re-enable tty echo on interrupt. The signal handler is
// transparent with system default, and immedeately releases the channel and
// calling the system sighandler after termios is set.
func EchoSigHandler(
	ctx context.Context,
	sigChan chan os.Signal,
	errChan chan error,
	term *unix.Termios) {
	for {
		var (
			sig       os.Signal
			sigRecved bool
		)
		select {
		case <-ctx.Done():
			errChan <- nil
			return
		case sig, sigRecved = <-sigChan:
		}
		if sig == unix.SIGWINCH || sig == unix.SIGURG {
			// Though SIGCHLD is ignored by default, in this context
			// we want to restore echo state.
			continue
		}
		// Restore Termios
		_ = unix.IoctlSetTermios(int(os.Stdin.Fd()), ioctlSetTermios, term)
		if sigRecved {
			switch sig {
			case unix.SIGCHLD:
				// SIGCHLD is expected when ssh terminates.
				errChan <- nil
			default:
				errChan <- errors.Errorf("Received signal: %s",
					unix.SignalName(sig.(unix.Signal)))
				// Relay signal to default handler
				_ = unix.Kill(os.Getpid(), sig.(unix.Signal))
			}
		} else {
			errChan <- nil
			return
		}
	}
}
