/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package types

import (
	"bytes"
	"compress/zlib"
	"io"
	"runtime"
	"sync"
)

// zlibWriters keep the writers between compressions: a reset writer compresses as a new one of
// the same level does, byte for byte, without allocating its tables again. The data of a
// transaction is signed binary that deflate does not shrink (310 bytes become 322 at the default
// level, in 26 times the time), so a writer stores it: the stream is zlib all the same, which every
// node reads as it reads a compressed one.
var zlibWriters = sync.Pool{New: func() any {
	w, _ := zlib.NewWriterLevel(nil, zlib.NoCompression)
	return w
}}

var zlibReaders sync.Pool

func DoZlibCompress(src []byte) []byte {
	var in bytes.Buffer
	w := zlibWriters.Get().(*zlib.Writer)
	w.Reset(&in)
	w.Write(src)
	w.Close()
	zlibWriters.Put(w)
	return in.Bytes()
}

func DoZlibUnCompress(compressSrc []byte) []byte {
	b := bytes.NewReader(compressSrc)
	var out bytes.Buffer
	var r io.ReadCloser
	if v := zlibReaders.Get(); v != nil {
		r = v.(io.ReadCloser)
		if r.(zlib.Resetter).Reset(b, nil) != nil {
			return out.Bytes()
		}
	} else {
		var err error
		if r, err = zlib.NewReader(b); err != nil {
			return out.Bytes()
		}
	}
	io.Copy(&out, r)
	r.Close()
	zlibReaders.Put(r)
	return out.Bytes()
}

// eachParallel runs f for each index below n on the processors of the node
func eachParallel(n int, f func(i int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := w; i < n; i += workers {
				f(i)
			}
		})
	}
	wg.Wait()
}
