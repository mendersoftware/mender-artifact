// Copyright 2016 Mender Software AS
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
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"syscall"
)

type Checksum struct {
	w io.Writer // underlying writer
	r io.Reader
	h hash.Hash
	c []byte // checksum
}

func NewWriterChecksum(w io.Writer) *Checksum {
	h := sha256.New()
	return &Checksum{
		w: io.MultiWriter(h, w),
		h: h,
	}
}

func NewReaderChecksum(r io.Reader) *Checksum {
	h := sha256.New()
	return &Checksum{
		r: io.TeeReader(r, h),
		h: h,
	}
}

func (c *Checksum) Write(p []byte) (int, error) {
	if c.w == nil {
		return 0, syscall.EBADF
	}
	return c.w.Write(p)
}

func (c *Checksum) Read(p []byte) (int, error) {
	if c.r == nil {
		return 0, syscall.EBADF
	}
	return c.r.Read(p)
}

func (c *Checksum) Checksum() []byte {
	sum := c.h.Sum(nil)
	checksum := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(checksum, c.h.Sum(nil))
	return checksum
}
