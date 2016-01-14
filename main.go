package main

import (
	"bytes"
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
)

var (
	remoteHost   = flag.String("remote-host", "localhost:2222", "ssh host to connect to")
	remoteListen = flag.String("remote-listen", "127.0.0.1:8080", "remote interface and port to serve on")
	sshUser      = flag.String("ssh-user", "core", "ssh user to use")
	manifest     = flag.String("manifest", "pods.yml", "file to serve on the remote connection")
)

func SSHAgent() ssh.AuthMethod {
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		return ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers)
	}
	return nil
}

func executeCmd(cmd, remoteHost string, config *ssh.ClientConfig) (string, error) {
	conn, err := ssh.Dial("tcp", remoteHost, config)
	if err != nil {
		return "", err
	}
	session, err := conn.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stdoutBuf
	session.Run(cmd)

	return remoteHost + ": " + stdoutBuf.String(), nil
}

func main() {
	flag.Parse()
	config := &ssh.ClientConfig{
		User: *sshUser,
		Auth: []ssh.AuthMethod{
			SSHAgent(),
		},
	}
	// Dial your ssh server.
	conn, err := ssh.Dial("tcp", *remoteHost, config)
	if err != nil {
		log.Fatalf("unable to connect: %s", err)
	}
	defer conn.Close()

	// Request the remote side to open port 8080 on all interfaces.
	l, err := conn.Listen("tcp", *remoteListen)
	if err != nil {
		log.Fatalf("unable to register tcp forward: %v", err)
	}
	defer l.Close()

	f, err := os.Open(*manifest)
	if err != nil {
		log.Fatalf("unable to open manifest: %v", err)
	}
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatalf("unable to read manifest: %v", err)
	}
	// Serve HTTP with your SSH server acting as a reverse proxy.
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		http.Serve(l, http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			fmt.Fprintf(resp, string(buf))
			wg.Done()
		}))
	}()

	cmd := fmt.Sprintf("/usr/bin/sudo kubelet --runonce=true --manifest-url=http://%s", *remoteListen)
	_, err = executeCmd(cmd, *remoteHost, config)
	if err != nil {
		log.Fatalf("unable to execute cmd %v: %v", cmd, err)
	}
	wg.Wait()
}
