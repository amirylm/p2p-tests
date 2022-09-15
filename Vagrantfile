# -*- mode: ruby -*-
# vi: set ft=ruby :

Vagrant.configure("2") do |config|

  config.vm.box = "ubuntu/focal64"

  config.disksize.size = '80GB'

  config.vm.synced_folder ".", "/vagrant", disabled: true
  config.vm.synced_folder ".", "/vagrant/p2p"

  config.vm.provider "virtualbox" do |vb|
    vb.memory = "8192"
    vb.cpus = 4
  end

  config.vm.network "private_network", type: "dhcp"

  config.vm.provision "shell", privileged: false, inline: <<-SHELL
    export DEBIAN_FRONTEND=noninteractive
    sudo apt-get update
    sudo apt-get upgrade -y

    # install dependencies
#     sudo apt-get install -qq -y make g++ python gcc-aarch64-linux-gnu \
#         apt-transport-https lsb-release ca-certificates gnupg git zip unzip bash curl 2> /dev/null
    sudo apt-get install -y git apt-transport-https ca-certificates curl software-properties-common make python g++ gcc-aarch64-linux-gnu bash
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add - 2> /dev/null
    sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu focal stable"
    apt-cache policy docker-ce
    sudo apt-get install -y docker-ce
    sudo usermod -aG docker vagrant
    # install go
    sudo snap install --classic --channel=1.18/stable go 2> /dev/null
    # install testground, TODO: extract to script
    cd /testground 2> /dev/null || (sudo mkdir /testground && \
                                    sudo docker pull iptestground/testground:edge && \
                                    sudo docker pull iptestground/sync-service:edge && \
                                    sudo docker pull iptestground/sidecar:edge && \
                                    sudo docker run -v /testground:/mount --rm --entrypoint cp iptestground/testground:edge /testground /mount/testground && \
                                    sudo chown vagrant: /testground && \
                                    sudo echo "export TESTGROUND_HOME=/testground" >> /home/vagrant/.bashrc && \
                                    sudo echo "alias testground=/testground/testground" >> /home/vagrant/.bashrc)
  SHELL
end
