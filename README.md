# monokube

monokube is a single binary that includes everything you need to run kubernetes  including apiserver, controller-manager, scheduler and etcd. It is different than hyperkube because all processes are ran in go-routines in a single binary, making it a complete working environment in one command. 

Additionally, monokube includes functionality that allows you to set up reverse ssh tunnels from your nodes to your locally running cluster (on your laptop), making it painless to setup a properly functioning cluster. This is particularly useful if you want to use kubernetes as an alternative to tools such as puppet or ansible for initial host bootstrapping. 

## Install

```
$ curl https://media.githubusercontent.com/media/polvi/monokube/master/bin/darwin/amd64/monokube > monokube
$ shasum monokube 
291e9ad46763cd2e2ac169f2295f668f3f47b603  monokube
$ chmod +x monokube
```

`kubectl` is included with binary too and can be accessed by creating a symlink to the original binary:

```
$ ln -s ./monokube ./kubectl 
```

## Basic Master


At this point `monokube` is your complete master control plane and can be started with:

```
$ ./monokube
...
```

Once running, test it out with kubectl

```
$ ./kubectl version
Client Version: version.Info{Major:"1", Minor:"1", GitVersion:"v1.1.4+$Format:%h$", GitCommit:"$Format:%H$", GitTreeState:"not a git tree"}
Server Version: version.Info{Major:"1", Minor:"1", GitVersion:"v1.1.4+$Format:%h$", GitCommit:"$Format:%H$", GitTreeState:"not a git tree"}
```

## Cluster setup

monokube can optionally setup ssh reverse proxy tunnels to nodes running the kubelet, making it painless to setup a secure cluster. Additionally it will invoke the kubelet with the right command line args to connect to the tunnel.

```
./monokube --nodes=host1.mylab:22,host2.mylab:22
```

### Example using Vagrant

Grab [coreos-vagrant](https://github.com/coreos/coreos-vagrant) and edit `config.rb` to set `$num_instances=3`, then from the `coreos-vagrant` directory:

```
vagrant up
```

`coreos-vagrant` will bring up the nodes with static IPs, so you can use them to target using the `--nodes` flag on `monokube`. 

```
./monokube --nodes=172.17.8.101:22,172.17.8.102:22,172.17.8.103:22
```

Now check that the nodes came up with `kubectl`

```
./kubectl get nodes
NAME           LABELS                                STATUS    AGE
172.17.8.101   kubernetes.io/hostname=172.17.8.101   Ready     1s
172.17.8.102   kubernetes.io/hostname=172.17.8.102   Ready     1s
172.17.8.103   kubernetes.io/hostname=172.17.8.103   Ready     0s
```
