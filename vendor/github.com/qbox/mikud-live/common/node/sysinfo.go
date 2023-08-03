package cnode

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"

	zlog "github.com/rs/zerolog"
)

type RebootStatus int

const (
	REBOOT_NO RebootStatus = iota
	REBOOT_YES
	REBOOT_UNKNOWN

	// REBOOT_CHECK_FILE_IN_MEM use memory file to check reboot
	REBOOT_CHECK_FILE_IN_MEM = "/dev/shm/pili-noded-reboot"
)

func GetUptime(xl *zlog.Logger) (uptime int64, rebootStatus RebootStatus, err error) {
	if runtime.GOOS == "darwin" {
		return 1000_000_000, REBOOT_NO, nil
	}

	uptimePath := "/proc/uptime"
	lines, err := ReadLines(uptimePath)
	if err != nil || len(lines) == 0 {
		xl.Warn().Msgf("ReadLines(\"/proc/uptime\") fail, err is %v or len(%v) == 0", err, len(lines))
		return
	}
	utSplit := strings.Split(lines[0], " ")
	if len(utSplit) != 2 {
		err = errors.New(fmt.Sprintf("len(%#v) != 2", utSplit))
		return
	}
	ut, err := strconv.ParseFloat(utSplit[0], 64)
	if err != nil {
		xl.Warn().Msgf("strconv.ParseFloat(%v, 64), err is %v", utSplit[0], err)
		return
	}
	uptime = int64(ut)

	rebootStatus = CheckReboot(xl, REBOOT_CHECK_FILE_IN_MEM)

	return
}

func CheckReboot(xl *zlog.Logger, name string) RebootStatus {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			if err = ioutil.WriteFile(name, []byte{}, 0644); err != nil {
				xl.Warn().Msgf("ioutil.WriteFile failed, err is %v", err)
				return REBOOT_UNKNOWN
			}
			return REBOOT_YES
		}
		xl.Warn().Msgf("os.IsNotExist(err) failed, err is %v", err)
		return REBOOT_UNKNOWN
	}
	return REBOOT_NO
}
