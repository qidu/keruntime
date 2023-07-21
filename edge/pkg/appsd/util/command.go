package util

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"k8s.io/klog/v2"
)

const (
	processShutdownTimeout = 15
)


func CheckCmdExists(cmd string) (bool, error) {
	_, err := exec.LookPath(cmd)
	if err != nil {
		klog.Errorf("cannot find command:%s\n", cmd)
		return false, err
	}
	return true, nil
}

func StartProcess(path string, arg string) error {
	var err error
	s := strings.Split(path, " ")
	newEnv := os.Environ()
	if s != nil && len(s) > 1 {
		for i := 0; i < len(s)-1; i++ {
			newEnv = append(newEnv, s[i])
		}
		path = s[len(s)-1]
	}
	if ok, err := CheckCmdExists(path); !ok {
		return err
	}
	args := strings.Split(arg," ")
	cmd := exec.Command(path, args...)
	cmd.Env = newEnv

	var stdin, stdout, stderr bytes.Buffer
	cmd.Stdin = &stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return err
	}
	outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	klog.Infof("exec command:%s,%s\n,out:%s\nerr:%s", path, args, outStr, errStr)
	return nil
}

func StopProcess(path string) error {
	s := strings.Split(path, " ")
	if s != nil && len(s) > 1 {
		path = s[len(s)-1]
	}

	processes, err := process.Processes()
	if err != nil {
		return err
	}
	
	var process *process.Process 
	for _, p := range processes {
		exePath, _ := p.Exe()
		if exePath == path {
			process = p
			break
		}
	}
	if process == nil {
		return fmt.Errorf("path %s is not exist", path)
	}

	var isRunning bool
	retry := 3
Loop:
	for retry > 0 {
		isRunning, _ = process.IsRunning()
		if !isRunning {
			break
		}
		err = syscall.Kill(int(process.Pid), syscall.SIGTERM)
		if err != nil {
			return err
		}
		// Wait up to 15secs for it to stop
		for i := time.Duration(0); i < processShutdownTimeout; i += time.Second {
			isRunning, _ = process.IsRunning()
			if !isRunning {
				break Loop
			}
			time.Sleep(time.Second)
		}
		retry--
	}
	if isRunning {
		err = syscall.Kill(int(process.Pid), syscall.SIGKILL)
		if err != nil {
			return err
		}
	}
	klog.Infof("stop process:%v success", path)
	return nil
}
