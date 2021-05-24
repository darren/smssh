package main

import (
	"io"
	"log"
	"os"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type idleReader struct {
	r       io.Reader
	buf     chan []byte
	timeout time.Duration
	err     error
}

func (ir *idleReader) read() {
	for {
		// 64的这个数值很小，后面的Read被调用是buf的大小应该不会小于64?
		buf := make([]byte, 64)
		n, err := ir.r.Read(buf)
		if err != nil {
			close(ir.buf)
			ir.err = err
			break
		}
		ir.buf <- buf[:n]
	}
}

func (ir *idleReader) Read(buf []byte) (n int, err error) {
	defer func() {
		if n > 0 {
			log.Printf(
				"[R-(%d/%d)]\n>-------------------------------------\n%q\n>-------------------------------------\n",
				n, cap(buf), buf[:n],
			)
		}
	}()

	select {
	case nbuf, ok := <-ir.buf:
		if !ok {
			return 0, ir.err
		}
		n = copy(buf, nbuf)
		if n < len(nbuf) {
			panic("under copy...")
		}
		return
	case <-time.After(ir.timeout):
		buf[0] = 0
		n = 1
		log.Printf("STDIN read timeout, sending anti idle...")
		return
	}
}

func newAntiIdleReader(timeout time.Duration) io.Reader {
	tf := encoding.HTMLEscapeUnsupported(simplifiedchinese.GBK.NewEncoder())
	stdin := transform.NewReader(os.Stdin, tf)

	ir := &idleReader{
		r:       stdin,
		buf:     make(chan []byte),
		timeout: timeout,
	}
	go ir.read()

	return ir
}
