F5 IPAM Controller
==================

The F5 IPAM Controller interfaces with an IPAM system to allocate IP addresses for host names in an orchestration environment.

The controller currently supports the following environments:

### Orchestrations:
[Kubernetes](https://kubernetes.io/)/[OpenShift](https://www.openshift.com/)

### IPAM systems:
[Infoblox](https://www.infoblox.com/)


Documentation
-------------

Official documentation coming soon...

Getting Help
------------

Details coming soon...

Contact F5 Technical support via your typical method for more time sensitive changes and other issues requiring immediate support.

Running
-------

Official docker images coming soon...

Building
--------

The official images are built using docker, but the adventurous can use standard go build tools.

### Official Build

Prerequisites:
- Docker

```shell
git clone https://github.com/f5networks/f5-ipam-ctlr.git
cd  f5-ipam-ctlr

# Use docker to build the release artifacts, into a local "_docker_workspace" directory, then put into docker images
# Alpine image
make prod

OR

# RHEL7 image
make prod BASE_OS=rhel7
```


### Alternate, unofficial build

A normal go and godep toolchain can be used as well

Prerequisites:
- go 1.9.4
- $GOPATH pointing at a valid go workspace
- godep (Only needed to modify vendor's packages)

```shell
mkdir -p $GOPATH/src/github.com/F5Networks
cd $GOPATH/src/github.com/F5Networks
git clone https://github.com/f5networks/f5-ipam-ctlr.git
cd f5-ipam-ctlr

# Build all packages, and run unit tests
make all test
```

To make changes to vendor dependencies, see [Devel](DEVEL.md)
