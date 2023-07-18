#!/usr/bin/env bash

# Copyright 2019 The KubeEdge Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# check if kubectl installed
function check_kubectl {
  echo "checking kubectl"
  command -v kubectl >/dev/null 2>&1
  if [[ $? -ne 0 ]]; then
    echo "installing kubectl ."
       curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
       curl -LO "https://dl.k8s.io/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl.sha256"
       echo "$(cat kubectl.sha256)  kubectl" | sha256sum --check >/dev/null 2>&1
       if [[ $? -ne 0 ]]; then 
	 echo "download the kubectl binary file has been destroyed, exiting."
	 exit 1
       fi
       install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
       command -v kubectl > /dev/null 2>&1
       if [[ $? -ne 0 ]]; then
	 echo "kind installed failed, exiting."
	 exit 1
       fi
  else
    echo -n "found kubectl, " && kubectl version --short --client
  fi
}

# check if kind installed
function check_kind {
  echo "checking kind"
  command -v kind >/dev/null 2>&1
  if [[ $? -ne 0 ]]; then
    echo "installing kind ."
    #GO111MODULE="on" go install sigs.k8s.io/kind@v0.12.0
    if [[ ! -f kind ]]; then
	 # For x86
         [ $(uname -m) = x86_64 ] && curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
         # For arm64
	 [ $(uname -m) = aarch64 ] && curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-arm64
    fi
    
    if [[ ! -f kind ]]; then
       echo "kind binary file download failed, exiting."
       exit 1
    fi
    
    chmod +x ./kind
    sudo mv ./kind /usr/local/bin/kind
    
    if [[ $? -ne 0 ]]; then
      echo "kind installed failed, exiting."
      exit 1
    fi

    # GOPATH=$(go env | grep GOPATH | awk -F= '{print $2}' | sed 's/\"//g')
    # export PATH=$PATH:$GOPATH/bin
    #cp $GOPATH/bin/kind /usr/local/bin/   
    command -v kind > /dev/null 2>&1 
   
    if [[ $? -ne 0 ]]; then 
      echo "cannot find kind command, kind installed failed, exiting."
      exit 1
    fi

    # avoid modifing go.sum and go.mod when installing the kind
    # git checkout -- go.mod go.sum
    
  else
    echo -n "found kind, version: " && kind version
  fi
}

# check if golangci-lint installed
function check_golangci-lint {
  GOPATH="${GOPATH:-$(go env GOPATH)}"
  echo "checking golangci-lint"
  export PATH=$PATH:$GOPATH/bin
  expectedVersion="1.42.0"
  command -v golangci-lint >/dev/null 2>&1
  if [[ $? -ne 0 ]]; then
    install_golangci-lint
  else
    version=$(golangci-lint version)
    if [[ $version =~ $expectedVersion ]]; then
      echo -n "found golangci-lint, version: " && golangci-lint version
    else
      echo "golangci-lint version not matched, now version is $version, begin to install new version $expectedVersion"
      install_golangci-lint
    fi
  fi
}

# check if mqtt installed
function check_mqtt {
  echo "checking mqtt"
  netstat -anp | grep 1883 > /dev/null 2>&1
  if [[ $? -ne 0 ]]; then
     echo "installing mqtt server in docker format"
     docker image pull eclipse-mosquitto:1.6.15
     mkdir /var/lib/mqtt
     cd /var/lib/mqtt && mkdir config data log
     chmod -R 664 /var/lib/mqtt 
     chmod -R 664 /var/lib/mqtt/log
     touch /var/lib/mqtt/config/mosquitto.conf
     echo "starting mqtt docker container"
     docker run -d --name mqttbroker --privileged \
	-p 1883:1883 -p 9001:9001 -v /var/lib/mqtt/config:/mosquitto/config \
	-v /var/lib/mqtt/data:/mosquitto/data -v /var/lib/mqtt/log:/mosquitto/log eclipse-mosquitto:1.6.15
     if [[ $? -ne 0 ]]; then 
	 echo "mqtt installed failed, exiting"
	 exit 1
     fi
     echo "mqtt docker container is running."
  fi
  command -v mosquitto_sub > /dev/null 2>&1
  if [[ $? -ne 0 ]]; then
     echo "installing mqtt client."
     apt-get install mosquitto-clients
  fi
}

function install_golangci-lint {
  echo "installing golangci-lint ."
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ${GOPATH}/bin v1.42.0
    if [[ $? -ne 0 ]]; then
      echo "golangci-lint installed failed, exiting."
      exit 1
    fi

    export PATH=$PATH:$GOPATH/bin
}

function verify_containerd_installed {
  # verify the containerd installed
  echo "checking containerd."
  command -v containerd >/dev/null
  if [[ $? -ne 0 ]]; then
      echo "installing the containerd ."
      sudo apt-get update
      # install dependency
      sudo apt-get install -y ca-certificates curl gnupg lsb-release
      sudo mkdir -p /etc/apt/keyrings
      curl -fsSL https://mirrors.aliyun.com/docker-ce/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
      # set apt repo
      echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://mirrors.aliyun.com/docker-ce/linux/ubuntu \
      $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
      sudo apt-get update
      sudo apt-get install -y containerd.io
      command -v containerd >/dev/null
      if [[ $? -ne 0 ]]; then
          echo "containerd installed failed, exiting."
          exit 1
      fi
  fi
}

function  verify_docker_installed {
  # verify the docker installed
  command -v docker >/dev/null 2>&1 && systemctl status docker | grep running
  if [[ $? -ne 0 ]]; then
    #cleanup_installed_docker
    #echo "must install the docker first"
    echo "installing the docker ."
    curl -fsSL get.docker.com -o get-docker.sh
    chmod +x get-docker.sh
    ./get-docker.sh --mirror Aliyun
    if [[ $? -ne 0 ]]; then
       echo "docker installed failed, exit."
       exit 1
    fi
    systemctl enable docker
    systemctl start docker
    sleep 2
    systemctl status docker | grep running
    if [[ $? -ne 0 ]]; then
       echo "docker daemon service is not running, exiting."
       exit 1
    fi
    echo "install docker success"
  else
    echo -n "found docker, version: " && docker version
  fi
}

# install CNI plugins
function install_cni_plugins() {
  # install CNI plugins if not exist
  if [ ! -f "/opt/cni/bin/loopback" ]; then
    echo -e "install CNI plugins..."
    mkdir -p /opt/cni/bin
    wget https://github.com/containernetworking/plugins/releases/download/v1.1.1/cni-plugins-linux-amd64-v1.1.1.tgz
    tar Cxzvf /opt/cni/bin cni-plugins-linux-amd64-v1.1.1.tgz
  else
    echo "CNI plugins already installed and no need to install"
  fi
}

function install_flannel() {
  echo "installing flannel plugin. "
  if [ ! -f  "flannel.yaml" ]; then 
    wget -O flannel.yaml  https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml
  fi

  if [ ! -f  "flannel.yaml" ]; then
    echo "download kube-flannel.yaml failed, exiting."
    exit 1
  fi
  kubectl apply -f flannel.yaml
  if [[ $? -ne 0 ]]; then
    echo "installed flannel plugin failed, exiting."
    exit 1
  fi
}

