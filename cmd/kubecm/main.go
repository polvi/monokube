package main

import (
	"bytes"
	"fmt"
	"github.com/coreos/etcd/etcdmain"
	"github.com/spf13/pflag"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/testutils"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	kubeapiserver "k8s.io/kubernetes/cmd/kube-apiserver/app"
	controller "k8s.io/kubernetes/cmd/kube-controller-manager/app"
	scheduler "k8s.io/kubernetes/plugin/cmd/kube-scheduler/app"

	"log"
	"net"
	"net/http"
	"os"
)

var (
	/*
		remoteHost   = flag.String("remote-host", "localhost:2222", "ssh host to connect to")
		remoteListen = flag.String("remote-listen", "127.0.0.1:8080", "remote interface and port to serve on")
		sshUser      = flag.String("ssh-user", "core", "ssh user to use")
		manifest     = flag.String("manifest", "pods.yml", "file to serve on the remote connection")
	*/
	remoteListen = "127.0.0.1:8080"
	sshUser      = "core"
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
	// weirdness because etcdmain uses os.Args and so I need to override for my flags
	mfs := pflag.NewFlagSet("main", pflag.ExitOnError)
	hosts := mfs.StringSlice("hosts", []string{}, "list of hosts to make part of cluster")
	mfs.Parse(os.Args)
	os.Args = []string{"arg0"}

	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			SSHAgent(),
		},
	}
	for _, remoteHost := range *hosts {
		// Dial your ssh server.
		go func(host string) {
			// Serve HTTP with your SSH server acting as a reverse proxy.
			go func() {
				for {
					conn, err := ssh.Dial("tcp", host, config)
					if err != nil {
						log.Fatalf("unable to connect: %s", err)
						return
					}
					defer conn.Close()

					// Request the remote side to open port 8080 on all interfaces.
					l, err := conn.Listen("tcp", remoteListen)
					if err != nil {
						log.Fatalf("unable to register tcp forward: %v", err)
						return
					}
					defer l.Close()
					fwd, _ := forward.New()
					http.Serve(l, http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
						req.URL = testutils.ParseURI("http://localhost:8080")
						fwd.ServeHTTP(resp, req)
					}))
					log.Println("proxy connection broken, reconnecting....")
				}
			}()
			go func() {
				// this will block, and the kubelet will stop once the connection is broken
				// loop for reconnection
				for {
					ip, _, err := net.SplitHostPort(host)
					if err != nil {
						log.Fatalf("unable split host port: %v", err)
						return
					}
					cmd := fmt.Sprintf("sudo /usr/bin/kubelet --hostname-override=%s --api-servers=http://localhost:8080", ip)
					_, err = executeCmd(cmd, host, config)
					if err != nil {
						log.Fatalf("unable to execute kubelet: %v", err)
						return
					}
					// if we got here something went wrong
					log.Println("kubelet connection broken, reconnecting....")
				}
			}()
			<-make(chan interface{})
		}(remoteHost)
	}

	go func() {
		etcdmain.Main()
	}()

	go func() {
		s := kubeapiserver.NewAPIServer()
		fs := pflag.NewFlagSet("apiserver", pflag.ContinueOnError)
		s.AddFlags(fs)
		fs.Parse([]string{"--service-cluster-ip-range=10.1.30.0/24", "--etcd-servers=http://localhost:2379", "--ssh-keyfile=/Users/polvi/.vagrant.d/insecure_private_key", "--ssh-user=core"})
		//fs.Parse([]string{"--service-cluster-ip-range=10.1.30.0/24", "--etcd-servers=http://localhost:2379"})
		s.Run([]string{})
	}()

	go func() {
		s := controller.NewCMServer()
		fs := pflag.NewFlagSet("controller", pflag.ContinueOnError)
		s.AddFlags(fs)
		fs.Parse([]string{})
		s.Run([]string{})
	}()

	go func() {
		s := scheduler.NewSchedulerServer()
		fs := pflag.NewFlagSet("scheduler", pflag.ContinueOnError)
		s.AddFlags(fs)
		fs.Parse([]string{})
		s.Run([]string{})
	}()
	<-make(chan interface{})
}
