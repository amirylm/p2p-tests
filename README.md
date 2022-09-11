# p2p-tests
Testground plans for p2p testing

## Installation

**Pre-requisites**

* [VirtualBox](https://www.virtualbox.org/wiki/Downloads)
* [Vagrant](https://www.vagrantup.com/downloads)
  * disksize plugin: \
    `$ vagrant plugin install vagrant-disksize`

**Machine Setup**

```shell
vagrant up
vagrant ssh
```

Within the VM, check that testground is configured and start the daemon:

```shell
testground version
testground daemon
```
