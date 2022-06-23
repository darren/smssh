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

func DialProxy(hostport string, config *ssh.ClientConfig, p *url.URL) (*ssh.Client, error) {
	var (
		dialer proxy.Dialer
		err    error
		auth   *proxy.Auth
	)

	if p.User != nil {
		auth = new(proxy.Auth)
		auth.User = p.User.Username()
		if p, ok := p.User.Password(); ok {
			auth.Password = p
		}
	}

	switch p.Scheme {
	case "socks5", "socks", "socks5h":
		dialer, err = proxy.SOCKS5(
			"tcp",
			p.Host,
			auth,
			&net.Dialer{
				Timeout: config.Timeout,
			})

		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", p.Scheme)

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
