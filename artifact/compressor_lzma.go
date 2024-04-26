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
//go:build !nolzma
// +build !nolzma

package artifact

import (
	"io"

	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

type CompressorLzma struct {
}

func NewCompressorLzma() Compressor {
	return &CompressorLzma{}
}

func (c *CompressorLzma) GetFileExtension() string {
	return ".xz"
}

func (c *CompressorLzma) NewReader(r io.Reader) (io.ReadCloser, error) {
	r, err := rc.NewReader(r)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(r), err
}

const xzDictSize = 64 * (1 << 20) // 64 MiB - dict size for preset -9
var (
	// Set write config as xz(1) -9
	wc = xz.WriterConfig{
		DictCap:  xzDictSize,
		CheckSum: xz.CRC64,
		// xz(1): --lzma2 mf:
		// The default depends on the preset: 0 uses hc3, 1–3 use hc4,
		// and the rest use bt4.
		// bt4 = BinaryTree with 4-byte hashing
		Matcher: lzma.BinaryTree,
		// xz(1): --block-size [...]
		// In multi-threaded mode [...] The default size is three times
		// the LZMA2 dictionary size or 1 MiB, whichever is more.
		BlockSize: 3 * xzDictSize,
	}
	rc = xz.ReaderConfig{DictCap: xzDictSize}
)

func (c *CompressorLzma) NewWriter(w io.Writer) (io.WriteCloser, error) {
	return wc.NewWriter(w)
}

func init() {
	RegisterCompressor("lzma", &CompressorLzma{})
}
