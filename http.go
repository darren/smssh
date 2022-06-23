package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/net/proxy"
)

type httpProxy struct {
	host     string
	username string
	password string
	forward  proxy.Dialer
}

func newHTTPProxy(dst *url.URL, forward proxy.Dialer) (proxy.Dialer, error) {
	s := new(httpProxy)
	s.host = dst.Host
	s.forward = forward
	if dst.User != nil {
		s.username = dst.User.Username()
		s.password, _ = dst.User.Password()
	}

	return s, nil
}

func (p *httpProxy) Dial(network, addr string) (net.Conn, error) {
	reqURL, err := url.Parse("http://" + addr)
	if err != nil {
		return nil, err
	}
	reqURL.Scheme = ""

	req, err := http.NewRequest("CONNECT", reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Close = false
	if p.username != "" {
		req.SetBasicAuth(p.username, p.password)
	}
	req.Header.Set("User-Agent", "smssh")

	conn, err := p.forward.Dial("tcp", p.host)
	if err != nil {
		return nil, err
	}

	err = req.Write(conn)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = fmt.Errorf("unexpected proxy reponse status: %d", resp.StatusCode)
		return nil, err
	}

	return conn, nil
}

func init() {
	proxy.RegisterDialerType("http", newHTTPProxy)
}
