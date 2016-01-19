# k9

k9 is a single binary that includes everything you need to run kubernetes locally. It also includes functionality that allows you to set up ssh tunnels to nodes, making it painless to setup a properly functioning cluster. This is particularly useful if you want to use kubernetes as an alternative to tools such as puppet or ansible for initial host bootstrapping. 

## Usage

```
$ wget http://...
$ chmod +x k9
```

`kubectl` is included with binary too and can be accessed by creating a symlink to the original binary:

```
$ ln -s ./k9 ./kubectl 
```

At this point `k9` is your complete master control plane and can be started with:

```
$ ./k9
...
```

Once running, test it out with kubectl

```
$ ./kubectl version
...
```
