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
```

Connect and check that testground is available:

```shell
vagrant ssh

$ testground help
```

Next, start testground daemon with `testground daemon`

...