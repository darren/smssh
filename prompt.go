package main

import (
	"bufio"
	"os"
	"strings"

	"golang.org/x/term"
)

func promptPassword(hint string) (string, error) {
	os.Stdout.WriteString(hint)
	buf, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(buf), err
}

func prompt(hint string) (string, error) {
	os.Stdout.WriteString(hint)
	rd := bufio.NewReader(os.Stdin)
	buf, err := rd.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(buf), nil
}
