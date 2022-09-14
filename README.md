# p2p-tests
Testground plans for p2p testing

## Links

* [getting started](https://docs.testground.ai/getting-started)
* [architecture docs](https://docs.testground.ai/concepts-and-architecture)
* [libp2p test plans](https://github.com/libp2p/test-plans)

## Installation

**VM**

Install [VirtualBox](https://www.virtualbox.org/wiki/Downloads) and
[Vagrant](https://www.vagrantup.com/downloads).

Install disksize plugin for vagrant: `vagrant plugin install vagrant-disksize`

Start with `vagrant up`

Login with `vagrant ssh` and check that testground is configured with `testground version`

**Desktop (docker)**

Pull images:

```shell
docker pull iptestground/testground:edge
docker pull iptestground/sync-service:edge
docker pull iptestground/sidecar:edge
```

Add `$TESTGROUND_HOME` env:

```shell
echo "export TESTGROUND_HOME=~/testground" >> .zshrc
source .zshrc
```

Run the daemon image

```shell
docker run -v "$TESTGROUND_HOME":/mount --rm --entrypoint cp iptestground/testground:edge /testground /mount/testground
```

Add `testground` alias:

```shell
echo "alias testground=$TESTGROUND_HOME/testground" >> .zshrc
source .zshrc
```

Check that testground is configured:

```shell
testground version
```

### Daemon

Start daemon with th following cmd:

**NOTE** that it blocks the terminal, open a new terminal with `vagrant ssh`

```shell
testground daemon
```

### Plans

**Import Plans**

```shell
# path (/vagrant/p2p/plans) should be changed in desktop
testground plan import --from /vagrant/p2p/plans
```

**Run Plans**

```shell
testground run single --plan=plans/topology \
                        --testcase=subnets \
                        --builder=exec:go \
                        --runner=local:exec \
                        --instances=50 \
```
