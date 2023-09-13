package cnode

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
)

const (
	PathDmiUUID          = "/sys/class/dmi/id/product_uuid"
	pathDbusMachineID    = "/var/lib/dbus/machine-id"
	pathSystemdMachineID = "/etc/machine-id"
	pathSystemdHostname  = "/etc/hostname"

	// 默认系统启动时间检查时间
	defaultRebootMin = 2
)

var (
	xl   zerolog.Logger
	once sync.Once

	machineID     = "www.unreachable.com"
	DefaultIDPath = "/home/qboxserver/.miku-nodeid"

	idConf NodeIDConf
)

type NodeIDConf struct {
	RebootMin int    `json:"reboot_min"`
	IDPath    string `json:"id_path"`
}

func init() {
	xl = zerolog.New(os.Stdout)

	idConf.IDPath = DefaultIDPath
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
	idConf.IDPath = DefaultIDPath
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

	// 先读取 dmi 类型信息（受系统保护，可能会读取失败）
	dmiUUID, err := slurpFile(PathDmiUUID)
	if err == nil {
		machineID += dmiUUID
	} else {
		xl.Error().Msgf("read /sys/class/dmi/id/product_uuid failed, err: %+v", err)
	}

	machineIDLocal, err := getMachineID()
	if err == nil {
		machineID += machineIDLocal
	} else {
		xl.Error().Msgf("read local machineID failed, err: %+v", err)
	}
	hostname, err := getHostname()
	if err == nil {
		machineID += hostname
	} else {
		xl.Error().Msgf("read local hostname failed, err: %+v", err)
	}

	if len(machineID) == 0 {
		panic("machineID get set failed.")
	}
}

func getMachineID() (string, error) {
	// 尝试读取受保护的 machine id
	var dbusMachineID string
	dbusMachineID, err := slurpFile(pathDbusMachineID)
	if err != nil {
		xl.Error().Msgf("read /var/lib/dbus/machine-id failed, err: %+v", err)
	}

	systemdMachineID, err := slurpFile(pathSystemdMachineID)
	if err != nil {
		xl.Error().Msgf("read /etc/machine-id failed, err: %+v", err)
		return "", err
	}
	dbusMachineID += systemdMachineID

	return dbusMachineID, err
}

// slurpFile read one-liner text files, strip newline.
func slurpFile(_path string) (string, error) {
	data, err := ioutil.ReadFile(_path)
	if err != nil {
		xl.Error().Msgf("read file %s failed, err: %s", _path, err.Error())
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
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
	hostname, err := getHostname()
	if err != nil {
		xl.Error().Msgf("get host name failed, err: %s", err.Error())
	} else {
		id = fmt.Sprintf("%s-%s", id, hostname)
	}
	return id
}

func getHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	hostname = strings.ToLower(hostname)
	hostname = replaceString(hostname)

	hostname = trimSuffix(hostname)
	return hostname, nil
}

var (
	regex = regexp.MustCompile(`[^a-zA-Z0-9]`)
)

func replaceString(input string) string {
	output := regex.ReplaceAllString(input, "-")
	return output
}

func trimSuffix(str string) string {
	str = strings.TrimSuffix(str, "-")
	if strings.HasSuffix(str, "-") {
		str = trimSuffix(str)
	}
	return str
}
