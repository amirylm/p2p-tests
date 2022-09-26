# p2p-tests
Testground plans for p2p testing

## Links

* [testground: getting started](https://docs.testground.ai/getting-started)
* [testground: architecture docs](https://docs.testground.ai/concepts-and-architecture)
* [libp2p test plans](https://github.com/libp2p/test-plans)

## Installation

### VM

**1.** Install [VirtualBox](https://www.virtualbox.org/wiki/Downloads) and
[Vagrant](https://www.vagrantup.com/downloads).

**2.** Install disksize plugin for vagrant: `vagrant plugin install vagrant-disksize`

**3.** Start with `vagrant up`

**4.** Login with `vagrant ssh` and check that testground is configured with `testground version`

Everything is ready, jump to [start daemon](#daemon).

### Mac (docker)

**1.** Pull related images:

```shell
docker pull iptestground/testground:edge
docker pull iptestground/sync-service:edge
docker pull iptestground/sidecar:edge
```

**2.** Add `$TESTGROUND_HOME` env:

```shell
echo "export TESTGROUND_HOME=~/testground" >> .zshrc
source .zshrc
```

**3.** Run testground container

```shell
docker run -v "$TESTGROUND_HOME":/mount --rm --entrypoint cp iptestground/testground:edge /testground /mount/testground
```

**4.** Add `testground` alias:

```shell
echo "alias testground=$TESTGROUND_HOME/testground" >> .zshrc
source .zshrc
```

**5.** Check that testground is configured:

```shell
testground version
```

## Daemon

Start daemon with th following cmd:

**NOTE** blocks the terminal, using vagrant you need to open a new terminal with `vagrant ssh`

```shell
testground daemon
```

## Plans

**Import Plans**

```shell
# path (/vagrant/p2p/plans) should be changed in desktop
testground plan import --from /vagrant/p2p/plans
```

**Run Plans**

```shell
testground run single --plan=plans/topology \
                        --testcase=groups \
                        --builder=exec:go \
                        --runner=local:exec \
                        --instances=100

# ----------------
                        
>>> Result:

Sep 13 09:04:03.650334  INFO    run is queued with ID: ccg4f0p4hr6rqnt7v670
```

**Collect Results**

The run ID can be used to collect results once finished:

```shell
cd /vagrant/p2p/data
testground collect --runner=local:docker <run-id>
tar zxvf <run-id>.tgz
```

## InfluxDB

Get into the container: `docker exec -it testground-influxdb /bin/bash`

Enter `influx` and create a user + verify it was created:
```
> CREATE USER admin WITH PASSWORD 'admin123' WITH ALL PRIVILEGES
> SHOW USERS
user  admin
----  -----
admin true
```

Enable HTTP:

```shell
apt update -y
apt-get install vim -y
vim /etc/influxdb/influxdb.conf
```

Paste the following inside `influxdb.conf`:

```
[http]
  enabled = true
  bind-address = ":8086"
  auth-enabled = true
```

Last thing, exit and restart the container:

`docker container restart testground-influxdb`


## Grafana 

**Setup**

1. Get the IP of the machine (`ip a`) and use it to open grafana in browser (http://machine-ip:3000) 

2. Configure influxdb as data source (`http://testground-influxdb:8086`) using http with the credentials from previous step

3. Import dashboard ([./resources/grafana_dashboard.json](/resources/grafana_dashboard.json))
