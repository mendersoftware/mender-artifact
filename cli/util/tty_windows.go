// Copyright 2023 Northern.tech AS
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

//go:build windows
// +build windows

package util

import (
	"context"
	"os"
	"os/signal"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// Disable TTY echo of stdin.
// Based on golang.org/x/crypto/ssh/terminal:util_windows.go
func DisableEcho(fd int) (uint32, error) {
	var cmode uint32
	err := windows.GetConsoleMode(windows.Handle(fd), &cmode)
	if err != nil {
		return 0, err
	}

	newCmode := cmode
	newCmode &^= (windows.ENABLE_ECHO_INPUT)
	newCmode |= (windows.ENABLE_LINE_INPUT |
		windows.ENABLE_PROCESSED_INPUT |
		windows.ENABLE_PROCESSED_OUTPUT)

	if err := windows.SetConsoleMode(windows.Handle(int(os.Stdin.Fd())), cmode); err != nil {
		return 0, err
	}
	return cmode, nil
}

// Signal handler to re-enable tty echo on interrupt. os/signal only
// handles ^C or ^BREAK events to the terminal, thus the signal won't be
// relayed to the OS handler for this case.
func EchoSigHandler(ctx context.Context, sigChan chan os.Signal, errChan chan error,
	cmode uint32) {
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
		// Enable console echo (restore cmode)
		windows.SetConsoleMode(windows.Handle(int(os.Stdin.Fd())), cmode)
		if sigRecved {
			signal.Stop(sigChan)
			errChan <- errors.Errorf("Received signal: %s",
				sig.String())
		} else {
			errChan <- nil
			return
		}
	}
}
