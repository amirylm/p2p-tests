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

Import plans and start daemon:

```shell
testground plan import --from /vagrant/p2p/plans
testground daemon
```

Run plans (from another shell)

```shell
testground run single --plan=plans/topology \
                        --testcase=subnets \
                        --builder=exec:go \
                        --runner=local:exec \
                        --instances=50 \
                        --test-param=
```