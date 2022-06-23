package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var (
	user      = flag.String("l", "", "login name")
	identity  = flag.String("i", "", "identity file ie: ~/.ssh/id_rsa")
	logfile   = flag.String("d", "", "enable debug and output logs to file")
	cloak     = flag.String("c", "", "cloak mode hide smth id")
	port      = flag.Int("p", 22, "port")
	forward   = flag.String("x", "", "proxy to use: socks5://127.0.0.1:1080")
	reconnect = flag.Bool("r", false, "auto reconnect after close")
)

func main() {
	flag.Parse()

	if *logfile != "" {
		go func() {
			http.ListenAndServe(":9191", nil)
		}()
		log.SetFlags(log.Lshortfile | log.LstdFlags)
		logf, err := os.OpenFile(*logfile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer logf.Close()
		log.SetOutput(logf)
	} else {
		log.SetOutput(ioutil.Discard)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	ctx, cancel := context.WithCancel(context.Background())

	fd := int(os.Stdin.Fd())
	termState, err := term.GetState(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	auth, err := getAuth()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	start := time.Now()
	retries := 0

	go func() {
		defer cancel()
		delay := 5 * time.Second
		for {
			retries++
			err := run(ctx, auth)

			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				if !*reconnect {
					return
				}

				delay *= 2
				fmt.Fprintf(os.Stderr, "Reconnect in %v\n", delay)
				time.Sleep(delay)
			} else {
				return
			}
		}
	}()

	select {
	case <-sig:
		cancel()
	case <-ctx.Done():
	}

	if err == nil {
		term.Restore(fd, termState)
	}

	if retries > 0 {
		fmt.Printf("Bye! Last connect time: %v, connects: %d\n", time.Since(start), retries)
	}
}

type Auth struct {
	user    string
	host    string
	port    int
	methods []ssh.AuthMethod
}

func getAuth() (*Auth, error) {
	var auth Auth
	var err error

	var methods []ssh.AuthMethod

	host := flag.Arg(0)
	if host == "" {
		host = "bbs.newsmth.net"
	} else {
		if i := strings.Index(host, "@"); i > 0 {
			*user = host[:i]
			if len(host[i:]) > 1 {
				host = host[i+1:]
			}
		}

		if i := strings.Index(host, ":"); i > 0 {
			if len(host[i:]) > 1 {
				*port, _ = strconv.Atoi(host[i+1:])
			}
			host = host[:i]
		}

	}
	auth.host = host
	auth.port = *port

	if *user == "" {
		*user, err = prompt("Username: ")
		if err != nil {
			return nil, err
		}
	}
	auth.user = *user

	if *identity == "" {
		var password string
		password, err = promptPassword("Password: ")
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.Password(password))
	} else {
		signer, err := signerFromFile(*identity)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	auth.methods = methods

	return &auth, nil
}

func run(ctx context.Context, auth *Auth) (err error) {
	idleTimeout := 10 * time.Minute

	config := &ssh.ClientConfig{
		User:            auth.user,
		Auth:            auth.methods,
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	config.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	hostport := fmt.Sprintf("%s:%d", auth.host, auth.port)

	var conn *ssh.Client

	if *forward != "" {
		if !strings.Contains(*forward, "://") {
			*forward = "socks://" + *forward
		}
		p, err := url.Parse(*forward)
		if err != nil {
			return fmt.Errorf("bad proxy: %v", err)
		}
		conn, err = DialProxy(hostport, config, p)
		idleTimeout = 1 * time.Minute
	} else {
		conn, err = Dial(hostport, config)
	}

	if err != nil {
		return fmt.Errorf("cannot connect %v: %v", hostport, err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("cannot open new session: %v", err)
	}
	defer session.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("terminal make raw: %s", err)
	}
	defer term.Restore(fd, state)

	w, h, err := term.GetSize(fd)
	if err != nil {
		return fmt.Errorf("terminal get size: %s", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	eterm := os.Getenv("TERM")
	if eterm == "" {
		eterm = "xterm-256color"
	}
	if err := session.RequestPty(eterm, h, w, modes); err != nil {
		return fmt.Errorf("session xterm: %s", err)
	}

	session.Stderr = os.Stderr
	session.Stdout = newReplaceWriter(*user)
	session.Stdin = newAntiIdleReader(idleTimeout)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGWINCH)
		for {
			<-sig

			w, h, err := term.GetSize(fd)
			if err != nil {
				log.Printf("Get window size failed %v", err)
				continue
			}

			err = session.WindowChange(h, w)
			if err != nil {
				log.Printf("Change window size failed %v", err)
			}
			log.Printf("WinSize Changed to %d %d", h, w)
		}
	}()

	if err := session.Shell(); err != nil {
		return fmt.Errorf("session shell: %s", err)
	}

	if err := session.Wait(); err != nil {
		if e, ok := err.(*ssh.ExitError); ok {
			switch e.ExitStatus() {
			case 1, 130:
				return nil
			}
		}
		return fmt.Errorf("ssh: %s", err)
	}
	return nil
}
