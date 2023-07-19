/*
Copyright 2018 The KubeEdge Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"k8s.io/klog/v2"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	uuid "github.com/satori/go.uuid"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/kubernetes/pkg/apis/core/validation"

	"github.com/kubeedge/kubeedge/common/constants"
)

const (
	// 主板序列号
	// 对于 docker 容器，不同容器内容也不相同
	productUuidPath = "/sys/class/dmi/id/product_uuid"
)

var (
	uuidName = "www.unreachable.com"
	defaultIDPath    = "/opt/qiniu/nodeid"
)

func GetLocalIP(hostName string) (string, error) {
	var ipAddr net.IP
	var err error

	// If looks up host failed, will use utilnet.ChooseHostInterface() below,
	// So ignore the error here
	addrs, _ := net.LookupIP(hostName)
	for _, addr := range addrs {
		if err := ValidateNodeIP(addr); err != nil {
			continue
		}
		if addr.To4() != nil {
			ipAddr = addr
			break
		}
		if ipAddr == nil && addr.To16() != nil {
			ipAddr = addr
		}
	}

	if ipAddr == nil {
		ipAddr, err = utilnet.ChooseHostInterface()
		if err != nil {
			return "", err
		}
	}
	return ipAddr.String(), nil
}

// ValidateNodeIP validates given node IP belongs to the current host
func ValidateNodeIP(nodeIP net.IP) error {
	// Honor IP limitations set in setNodeStatus()
	if nodeIP.To4() == nil && nodeIP.To16() == nil {
		return fmt.Errorf("nodeIP must be a valid IP address")
	}
	if nodeIP.IsLoopback() {
		return fmt.Errorf("nodeIP can't be loopback address")
	}
	if nodeIP.IsMulticast() {
		return fmt.Errorf("nodeIP can't be a multicast address")
	}
	if nodeIP.IsLinkLocalUnicast() {
		return fmt.Errorf("nodeIP can't be a link-local unicast address")
	}
	if nodeIP.IsUnspecified() {
		return fmt.Errorf("nodeIP can't be an all zeros address")
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil && ip.Equal(nodeIP) {
			return nil
		}
	}
	return fmt.Errorf("node IP: %q not found in the host's network interfaces", nodeIP.String())
}

//Command executes command and returns output
func Command(name string, arg []string) (string, error) {
	cmd := exec.Command(name, arg...)
	ret, err := cmd.Output()
	if err != nil {
		return string(ret), err
	}
	return strings.Trim(string(ret), "\n"), nil
}

//GetCurPath returns filepath
func GetCurPath() string {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	rst := filepath.Dir(path)
	return rst
}

func SpliceErrors(errors []error) string {
	if len(errors) == 0 {
		return ""
	}
	var stb strings.Builder
	stb.WriteString("[\n")
	for _, err := range errors {
		stb.WriteString(fmt.Sprintf("  %s\n", err.Error()))
	}
	stb.WriteString("]\n")
	return stb.String()
}

// GetHostname returns a reasonable hostname
func GetHostname() string {
	hostnameOverride, err := os.Hostname()
	if err != nil {
		return constants.DefaultHostnameOverride
	}
	msgs := validation.ValidateNodeName(hostnameOverride, false)
	if len(msgs) > 0 {
		return constants.DefaultHostnameOverride
	}
	return hostnameOverride
}

// ConcatStrings use bytes.buffer to concatenate string variable
func ConcatStrings(ss ...string) string {
	var bff bytes.Buffer
	for _, s := range ss {
		bff.WriteString(s)
	}
	return bff.String()
}

// GetNodeId used to obtain the current node ID after service startup
// node id keep unchanged after service startup
func GetNodeId() (string, error) {
	var id string
	// read for the fixed path, it will be treated as a new node when path is not exist
	idf, err := os.Open(defaultIDPath)
	if os.IsNotExist(err) {
		id, err = genUUID()
		if err != nil {
			klog.Errorf("gen uuid failed, err: %v", err)
			return "", err
		}
		idf, err = newIDFile(id)
		if err != nil {
			klog.Errorf("read id file info failed, err: %v", err)
			return "", err
		}
	}
	defer idf.Close()

	scanner := bufio.NewReader(idf)
	lines, _, err := scanner.ReadLine()

	if err != nil {
		if err != io.EOF {
			klog.Errorf("read id info failed, err: %v", err)
			return "", err
		}
	}
	if len(lines) == 0 {
		klog.Error("empty id info")
	}

	return strings.TrimSuffix(string(lines), "\n"), nil
}

func newIDFile(id string) (*os.File, error) {
	err := createIDFile(id)
	if err != nil {
		return nil, err
	}
	// ensure the file was being created
	f, err := os.Open(defaultIDPath)
	if err != nil {
		klog.Errorf("reopen id file failed, err: %v", err)
		return nil, err
	}
	return f, nil
}

func createIDFile(name string) error {
	err := os.MkdirAll(path.Dir(defaultIDPath), os.ModePerm)
	if err != nil {
		klog.Errorf("create id file path failed, err: %v", err)
		return err
	}
	f, err := os.Create(defaultIDPath)
	if err != nil {
		klog.Errorf("create id file failed, err: %v", err)
		return err
	}
	_, err = f.WriteString(name)
	if err != nil {
		return err
	}
	// commit
	err = f.Sync()
	if err != nil {
		klog.Errorf("sync id fo sys failed, err: %v", err)
		return err
	}
	err = f.Close()

	return err
}

func genUUID() (string, error) {
	bs, err := os.ReadFile(productUuidPath)
	if err != nil {
		klog.Errorf("read product uuid failed, err: %v", err)
		return "", err
	}
	uuidName = strings.TrimSuffix(string(bs), "\n")
	name := uuid.NewV3(uuid.NamespaceDNS, uuidName)
	return name.String(), nil
}
