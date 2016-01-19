package main

import (
	"bytes"
	"fmt"
	"github.com/coreos/etcd/etcdmain"
	"github.com/jpillora/backoff"
	"github.com/spf13/pflag"
	"github.com/vulcand/oxy/forward"
	"github.com/vulcand/oxy/testutils"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	kubeapiserver "k8s.io/kubernetes/cmd/kube-apiserver/app"
	controller "k8s.io/kubernetes/cmd/kube-controller-manager/app"
	"k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	scheduler "k8s.io/kubernetes/plugin/cmd/kube-scheduler/app"
	"path/filepath"
	"time"

	"log"
	"net"
	"net/http"
	"os"
)

var (
	remoteListen   = "127.0.0.1:8080"
	sshUser        = "core"
	sshKeyfile     = "/Users/polvi/.vagrant.d/insecure_private_key"
	clusterIPRange = "10.1.30.0/24"
)

func PublicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
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
	// embed kubectl
	if filepath.Base(os.Args[0]) == "kubectl" {
		cmd := cmd.NewKubectlCommand(cmdutil.NewFactory(nil), os.Stdin, os.Stdout, os.Stderr)
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		return
	}

	mfs := pflag.NewFlagSet("main", pflag.ExitOnError)
	nodes := mfs.StringSlice("nodes", []string{}, "list of nodes to make part of cluster")
	mfs.Parse(os.Args)

	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			PublicKeyFile(sshKeyfile),
		},
	}
	for _, remoteHost := range *nodes {
		// Dial your ssh server.
		go func(host string) {
			// Serve HTTP with your SSH server acting as a reverse proxy.
			go func() {
				b := &backoff.Backoff{
					//These are the defaults
					Min:    100 * time.Millisecond,
					Max:    10 * time.Second,
					Factor: 2,
					Jitter: false,
				}
				for {
					conn, err := ssh.Dial("tcp", host, config)
					if err != nil {
						log.Println("unable to connect, retrying:", err)
						time.Sleep(b.Duration())
						continue
					}
					defer conn.Close()

					// Request the remote side to open port 8080 on all interfaces.
					l, err := conn.Listen("tcp", remoteListen)
					if err != nil {
						log.Println("unable to register tcp forward, retrying:", err)
						time.Sleep(b.Duration())
						continue
					}
					defer l.Close()
					fwd, _ := forward.New()
					http.Serve(l, http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
						req.URL = testutils.ParseURI("http://localhost:8080")
						fwd.ServeHTTP(resp, req)
					}))
					log.Println("proxy connection broken, reconnecting....")
					time.Sleep(b.Duration())
				}
			}()
			go func() {
				// this will block, and the kubelet will stop once the connection is broken
				// loop for reconnection
				b := &backoff.Backoff{
					//These are the defaults
					Min:    100 * time.Millisecond,
					Max:    10 * time.Second,
					Factor: 2,
					Jitter: false,
				}

				for {
					ip, _, err := net.SplitHostPort(host)
					if err != nil {
						log.Fatalf("unable split host port: %v", err)
						return
					}
					cmd := fmt.Sprintf("sudo /usr/bin/kubelet --hostname-override=%s --api-servers=http://localhost:8080", ip)
					_, err = executeCmd(cmd, host, config)
					if err != nil {
						log.Println("unable to execute kubelet, retrying:", err)
					}
					// if we got here something went wrong
					dur := b.Duration()
					log.Println("kubelet connection broken, reconnecting in", dur)
					time.Sleep(dur)

				}
			}()
			<-make(chan interface{})
		}(remoteHost)
	}

	go func() {
		// etcd reads os.Args so we have to use mess with them
		os.Args = []string{"etcd"}
		etcdmain.Main()
	}()

	go func() {
		s := kubeapiserver.NewAPIServer()
		fs := pflag.NewFlagSet("apiserver", pflag.ContinueOnError)
		s.AddFlags(fs)
		fs.Parse([]string{
			"--service-cluster-ip-range=" + clusterIPRange,
			"--etcd-servers=http://127.0.0.1:2379",
			"--ssh-keyfile=" + sshKeyfile,
			"--ssh-user=" + sshUser,
		})
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
