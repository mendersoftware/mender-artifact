// Copyright 2020 Northern.tech AS
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

package artifact

import (
	"archive/tar"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// sourceDateEpochVar is the reproducible-builds convention for pinning
// timestamps embedded in build output.
// See https://reproducible-builds.org/docs/source-date-epoch/.
const sourceDateEpochVar = "SOURCE_DATE_EPOCH"

// sourceDateEpoch reports the timestamp requested by SOURCE_DATE_EPOCH, and
// whether the variable was set at all. A malformed value is an error rather
// than a silent fallback: a build that believes it is reproducible and is not
// is worse than one that stops.
func sourceDateEpoch() (time.Time, bool, error) {
	v, ok := os.LookupEnv(sourceDateEpochVar)
	if !ok {
		return time.Time{}, false, nil
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return time.Time{}, false, errors.Wrapf(err,
			"arch: invalid %s value %q", sourceDateEpochVar, v)
	}
	return time.Unix(secs, 0).UTC(), true, nil
}

type FileArchiver struct {
	*tar.Writer
}

func NewTarWriterFile(tw *tar.Writer) *FileArchiver {
	w := FileArchiver{
		Writer: tw,
	}
	return &w
}

func (fa *FileArchiver) Write(f *os.File, archivePath string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return errors.Wrapf(err, "arch: invalid file info header")
	}
	hdr.Name = archivePath

	// This writer archives both the temporaries created during the run
	// (header.tar.gz, data/NNNN.tar.gz) and the caller's own payload files
	// and state scripts. tar.FileInfoHeader copies each file's mtime, which
	// for the temporaries changes on every invocation, so when
	// SOURCE_DATE_EPOCH is set the timestamps are pinned to it. Ownership is
	// left as the file has it: SOURCE_DATE_EPOCH only covers timestamps.
	epoch, ok, err := sourceDateEpoch()
	if err != nil {
		return err
	}
	if ok {
		hdr.ModTime = epoch
		hdr.AccessTime = time.Time{}
		hdr.ChangeTime = time.Time{}
	}

	if err = fa.Writer.WriteHeader(hdr); err != nil {
		return errors.Wrapf(err, "arch: error writing header")
	}

	if _, err := io.Copy(fa.Writer, f); err != nil {
		return errors.Wrapf(err, "writer: can not tar header")
	}
	return nil
}
