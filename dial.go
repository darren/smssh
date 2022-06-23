package main

import (
	"fmt"
	"net"
	"net/url"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

func Dial(hostport string, config *ssh.ClientConfig) (*ssh.Client, error) {
	return ssh.Dial("tcp", hostport, config)
}

func DialProxy(hostport string, config *ssh.ClientConfig, pURL string) (*ssh.Client, error) {
	var (
		dialer proxy.Dialer
		err    error
		auth   *proxy.Auth
	)

	p, err := url.Parse(pURL)
	if err != nil {
		return nil, fmt.Errorf("bad proxy: %v", err)
	}

	if p.User != nil {
		auth = new(proxy.Auth)
		auth.User = p.User.Username()
		if p, ok := p.User.Password(); ok {
			auth.Password = p
		}
	}

	dialer, err = proxy.FromURL(p, &net.Dialer{
		Timeout: config.Timeout,
	})
	if err != nil {
		return nil, err
	}
	conn, err := dialer.Dial("tcp", hostport)
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, hostport, config)
	if err != nil {
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}
