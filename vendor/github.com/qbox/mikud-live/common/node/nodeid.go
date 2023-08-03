package cnode

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
)

const (
	pathSystemdHostname = "/etc/hostname"

	//pathSystemdMachineID = "/etc/machine-id"
	//pathDbusMachineID    = "/var/lib/dbus/machine-id"

	// 默认系统启动时间检查时间
	defaultRebootMin = 2
	defaultIDPath    = "/home/qboxserver/.miku-nodeid"
)

var (
	xl   zerolog.Logger
	once sync.Once

	machineID = "www.unreachable.com"

	idConf NodeIDConf
)

type NodeIDConf struct {
	RebootMin int    `json:"reboot_min"`
	IDPath    string `json:"id_path"`
}

func init() {
	xl = zerolog.New(os.Stdout)

	idConf.IDPath = defaultIDPath
	idConf.RebootMin = defaultRebootMin

	if runtime.GOOS == "darwin" {
		return
	}

	getSetMachineID()
	machineID = strings.TrimSuffix(machineID, "\n")
}

func SetIDConf(conf NodeIDConf) {
	once.Do(func() {
		idConf.IDPath = conf.IDPath
		idConf.RebootMin = conf.RebootMin
	})
}

func ReSetDefaultLogger(writers []io.Writer) {
	xl = zerolog.New(zerolog.MultiLevelWriter(writers...))
}

// GetNodeId 用于服务启动时获取当前节点id
// 服务进程启动后 node id 是保持不变的
func GetNodeId() string {
	uptime, _, err := GetUptime(&xl)
	if err != nil {
		xl.Panic().Msg(err.Error())
	}

	// 检测系统是否启动足够时间
	if int(uptime) < idConf.RebootMin*60 {
		xl.Panic().Msgf("reboot time is not enough, uptime: %d", uptime)
	}

	var id string
	// 从固定路径读取，路径不存在视为新节点
	idf, err := os.Open(idConf.IDPath)
	if os.IsNotExist(err) {
		id = genNodeID()
		idf, err = newIDFile(id)
	}
	if err != nil {
		xl.Panic().Msgf("read id file info failed, err: %v", err)
	}
	defer idf.Close()

	scanner := bufio.NewReader(idf)
	lines, _, err := scanner.ReadLine()

	xl.Debug().Msgf("id file content: %s", string(lines))
	if err != nil {
		xl.Warn().Msgf("read id info failed, err: %v", err)
		if err != io.EOF {
			xl.Panic().Msgf("read id info failed, err: %v", err)
		}

		err = nil
	}
	if len(lines) == 0 {
		xl.Panic().Msg("empty id info")
	}

	return strings.TrimSuffix(string(lines), "\n")
}

// getSetMachineID read the `hostname` info
func getSetMachineID() {
	systemdMachineID := slurpFile(pathSystemdHostname)

	if systemdMachineID != "" {
		machineID = systemdMachineID
		return
	}

	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		xl.Panic().Msgf("failed to gen rand seed, err: %s", err.Error())
		return
	}
	newMachineID := fmt.Sprintf("%x%x", random, time.Now().Unix())

	spewFile(pathSystemdHostname, newMachineID, 0444)
	machineID = newMachineID
}

// slurpFile read one-liner text files, strip newline.
func slurpFile(_path string) string {
	data, err := ioutil.ReadFile(_path)
	if err != nil {
		xl.Error().Msgf("read file %s failed, err: %s", _path, err.Error())
		return ""
	}

	return strings.TrimSpace(string(data))
}

// spewFile write one-liner text files, add newline, ignore errors (best effort).
func spewFile(_path string, data string, perm os.FileMode) {
	err := ioutil.WriteFile(_path, []byte(data+"\n"), perm)
	if err != nil {
		xl.Error().Msgf("write file %s failed, err: %s", _path, err.Error())
	}
}

func newIDFile(id string) (*os.File, error) {
	err := createIDFile(id)
	if err != nil {
		return nil, err
	}

	// ensure the file was being created
	f, err := os.Open(idConf.IDPath)
	if err != nil {
		xl.Panic().Msgf("reopen id file failed, err: %v", err)
	}
	return f, nil
}

func createIDFile(name string) error {

	err := os.MkdirAll(path.Dir(idConf.IDPath), os.ModePerm)
	if err != nil {
		xl.Panic().Msgf("create id file path failed, err: %v", err)
	}
	f, err := os.Create(idConf.IDPath)
	if err != nil {
		xl.Panic().Msgf("create id file failed, err: %v", err)
	}
	_, err = f.WriteString(name)
	if err != nil {
		return err
	}

	// commit
	err = f.Sync()
	if err != nil {
		xl.Panic().Msgf("sync id fo sys failed, err: %v", err)
	}
	err = f.Close()

	return err
}

func GenUUID() string {
	name := uuid.NewV3(uuid.NamespaceDNS, machineID)
	return name.String()
}

func genNodeID() string {
	var id = GenUUID()
	hostname, err := os.Hostname()
	if err != nil {
		xl.Error().Msgf("get host name failed, err: %s", err.Error())
	} else {
		id = fmt.Sprintf("%s-%s", id, hostname)
	}
	return id
}
