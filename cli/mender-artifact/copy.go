// Copyright 2019 Northern.tech AS
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

package main

import (
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/urfave/cli"
)

var isimg = regexp.MustCompile(`\.(mender|sdimg|uefiimg)`)

func Cat(c *cli.Context) (err error) {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError("compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(), 1)
	}
	if c.NArg() != 1 {
		return cli.NewExitError(fmt.Sprintf("Got %d arguments, wants one", c.NArg()), 1)
	}
	if !isimg.MatchString(c.Args().First()) {
		return cli.NewExitError("The input image does not seem to be a valid image", 1)
	}

	privateKey, err := getKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Unable to load key: "+err.Error(), 1)
	}

	r, err := virtualImage.OpenFile(comp, privateKey, c.Args().First())
	defer func() {
		if r == nil {
			return
		}
		cerr := r.Close()
		if err == nil {
			err = cerr
		}
	}()
	if err != nil {
		return cli.NewExitError(fmt.Sprintf("failed to open the partition reader: err: %v", err), 1)
	}
	var w io.WriteCloser = os.Stdout
	if _, err = io.Copy(w, r); err != nil {
		return cli.NewExitError(fmt.Sprintf("failed to copy from: %s to stdout: err: %v", c.Args().First(), err), 1)
	}
	return nil
}

func Copy(c *cli.Context) (err error) {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError("compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(), 1)
	}

	privateKey, err := getKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Unable to load key: "+err.Error(), 1)
	}

	var r io.ReadCloser
	var w io.WriteCloser
	wclose := func(w io.Closer) {
		if w == nil {
			return
		}
		cerr := w.Close()
		if err == nil {
			err = cerr
		}
	}
	var vfile VPFile
	switch parseCLIOptions(c) {
	case copyin:
		r, err = os.Open(c.Args().First())
		defer r.Close()
		if err != nil {
			return cli.NewExitError(err, 1)
		}
		vfile, err = virtualImage.OpenFile(comp, privateKey, c.Args().Get(1))
		defer wclose(vfile)
		if err != nil {
			return cli.NewExitError(err, 1)
		}
		if err = vfile.CopyTo(c.Args().First()); err != nil {
			return cli.NewExitError(err, 1)
		}
		return nil
	case copyinstdin:
		r = os.Stdin
		vfile, err = virtualImage.OpenFile(comp, privateKey, c.Args().Get(1))
		defer wclose(vfile)
		if err != nil {
			return cli.NewExitError(err, 1)
		}
		w = vfile
	case copyout:
		vfile, err = virtualImage.OpenFile(comp, privateKey, c.Args().First())
		defer wclose(vfile)
		if err != nil {
			return cli.NewExitError(err, 1)
		}
		if err = vfile.CopyFrom(c.Args().Get(1)); err != nil {
			return cli.NewExitError(fmt.Sprintf("%v", err), 1)
		}
		return nil
	case parseError:
		return cli.NewExitError(fmt.Sprintln("no artifact or sdimage provided"), 1)
	case argerror:
		return cli.NewExitError(fmt.Sprintf("got %d arguments, wants two", c.NArg()), 1)
	default:
		return cli.NewExitError("critical error", 1)
	}

	_, err = io.Copy(w, r)
	if err != nil {
		return cli.NewExitError(err, 1)
	}
	return nil
}

// Install installs a file from the host filesystem onto either
// a mender artifact, or an sdimg.
func Install(c *cli.Context) (err error) {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError("compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(), 1)
	}

	privateKey, err := getKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Unable to load key: "+err.Error(), 1)
	}

	var r io.ReadCloser
	var w io.WriteCloser
	wclose := func(w io.Closer) {
		if w == nil {
			return
		}
		cerr := w.Close()
		if err == nil {
			err = cerr
		}
	}
	switch parseCLIOptions(c) {
	case copyin:
		var perm os.FileMode
		if c.Int("mode") == 0 {
			return cli.NewExitError("File permissions needs to be set, if you are simply copying, the cp command should fit your needs", 1)
		}
		perm = os.FileMode(c.Int("mode"))
		r, err = os.OpenFile(c.Args().First(), os.O_RDWR, perm)
		defer r.Close()
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("%v", err), 1)
		}
		f, err := virtualImage.OpenFile(comp, privateKey, c.Args().Get(1))
		defer wclose(f)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("%v", err), 1)
		}
		w = f
		if _, err = io.Copy(w, r); err != nil {
			return cli.NewExitError(fmt.Sprintf("%v", err), 1)
		}
		return nil
	case parseError:
		return cli.NewExitError("No artifact or sdimg provided", 1)
	case argerror:
		return cli.NewExitError(fmt.Sprintf("got %d arguments, wants two", c.NArg()), 1)
	default:
		return cli.NewExitError("Unrecognized parse error", 1)
	}
}

func Remove(c *cli.Context) (err error) {
	wclose := func(w io.Closer) {
		if w == nil {
			return
		}
		cerr := w.Close()
		if err == nil {
			err = cerr
		}
	}

	privateKey, err := getKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Unable to load key: "+err.Error(), 1)
	}

	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError("compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(), 1)
	}
	if c.NArg() != 1 {
		return cli.NewExitError(fmt.Sprintf("Got %d arguments, wants one", c.NArg()), 1)
	}
	if !isimg.MatchString(c.Args().First()) {
		return cli.NewExitError("The input image does not have a valid extension", 1)
	}
	f, err := virtualImage.OpenFile(comp, privateKey, c.Args().First())
	defer wclose(f)
	if err != nil {
		return cli.NewExitError(fmt.Sprintf("failed to open the partition reader: err: %v", err), 1)
	}
	return f.Delete(c.Bool("recursive"))
}

// enumerate cli-options
const (
	copyin = iota
	copyinstdin
	copyout
	parseError
	argerror
	criterror
)

func parseCLIOptions(c *cli.Context) int {

	if c.NArg() != 2 {
		return argerror
	}

	// If the first argument is '-', read from stdin
	if c.Args().First() == "-" {
		// Read from stdin
		if !isimg.MatchString(c.Args().Get(1)) {
			return parseError
		}
		return copyinstdin
	}

	switch {

	case isimg.MatchString(c.Args().First()):
		return copyout

	case isimg.MatchString(c.Args().Get(1)):
		return copyin

	default:
		return parseError
	}
}
