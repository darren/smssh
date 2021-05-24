package main

import (
	"io"
	"log"
	"os"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type loggerWriter struct {
	io.Writer
}

func (rw *loggerWriter) Write(buf []byte) (int, error) {
	log.Printf(
		"[W-(%d)]\n<-------------------------------------\n%q\n<-------------------------------------\n",
		len(buf),
		string(buf),
	)

	return rw.Writer.Write(buf)
}

type replaceWriter struct {
	io.Writer
	seeker *Seeker
	cloak  *Cloak
}

func (rw *replaceWriter) Write(buf []byte) (n int, err error) {
	n = len(buf)
	if rw.seeker != nil {
		buf = rw.seeker.ReplaceIP(buf)
	}

	if rw.cloak != nil {
		buf = rw.cloak.Replace(buf)
	}

	_, err = rw.Writer.Write(buf)
	return
}

func newReplaceWriter(username string) io.Writer {
	seeker, err := NewSeeker("qqwry.dat")
	if err != nil {
		log.Printf("load ip data failed %v", err)
	}

	stdout := transform.NewWriter(
		&loggerWriter{os.Stdout},
		simplifiedchinese.GBK.NewDecoder().Transformer,
	)

	var cloaker *Cloak
	if *cloak != "" {
		cloaker, err = newCloak(username, *cloak)
		if err != nil {
			panic(err)
		}
	}

	return &replaceWriter{
		Writer: stdout,
		seeker: seeker,
		cloak:  cloaker,
	}
}
