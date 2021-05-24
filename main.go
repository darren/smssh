package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var (
	user     = flag.String("l", "", "login_name")
	identity = flag.String("i", "", "identity file")
	logfile  = flag.String("d", "", "debug log")
	cloak    = flag.String("c", "", "cloak mode hide smth id")
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

	go func() {
		if err := run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		cancel()
	}()

	select {
	case <-sig:
		cancel()
	case <-ctx.Done():
	}
}

func run(ctx context.Context) (err error) {
	var methods []ssh.AuthMethod

	if *user == "" {
		*user, err = prompt("Username: ")
		if err != nil {
			return err
		}
	}

	if *identity == "" {
		var password string
		password, err = promptPassword("Password: ")
		if err != nil {
			return err
		}
		methods = append(methods, ssh.Password(password))
	} else {
		signer, err := signerFromFile(*identity)
		if err != nil {
			return err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	config := &ssh.ClientConfig{
		User:            *user,
		Auth:            methods,
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	config.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	host := flag.Arg(0)
	if host == "" {
		host = "bbs.newsmth.net"
	}
	hostport := fmt.Sprintf("%s:22", host)
	conn, err := ssh.Dial("tcp", hostport, config)
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
	session.Stdin = newAntiIdleReader(10 * time.Minute)

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
			case 130:
				return nil
			}
		}
		return fmt.Errorf("ssh: %s", err)
	}
	return nil
}
