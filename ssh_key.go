package main

import (
	"fmt"
	"io/ioutil"

	"golang.org/x/crypto/ssh"
)

func signerFromFile(filename string) (ssh.Signer, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(buf)
	if err != nil {
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			password, err := promptPassword("Password: ")
			if err != nil {
				return nil, err
			}
			return ssh.ParsePrivateKeyWithPassphrase(buf, []byte(password))
		}
		return nil, fmt.Errorf("parse key %s failed %v", filename, err)
	}
	return signer, nil
}
