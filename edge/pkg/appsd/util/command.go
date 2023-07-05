package util

import (
	"bytes"
	"os/exec"

	"github.com/shirou/gopsutil/v3/process"
	"k8s.io/klog/v2"
)

func CheckCmdExists(cmd string) (bool, error) {
	_, err := exec.LookPath(cmd)
	if err != nil {
		klog.Errorf("cannot find command:%s\n", cmd)
		return false, err
	}
	return true, nil
}

func StartProcess(path, args string) error {
	var err error
	if ok, err := CheckCmdExists(path); !ok {
		return err
	}
	cmd := exec.Command(path, args)
	var stdin, stdout, stderr bytes.Buffer
	cmd.Stdin = &stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	cmd.Start()
	if err != nil {
		return err
	}
	outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	klog.Infof("exec command:%s,%s\n,out:%s\nerr:%s", path, args, outStr, errStr)
	return nil
}

func StopProcess(path string) error {
	processes, err := process.Processes()
	if err != nil {
		return err
	}
	for _, process := range processes {
		exePath, _ := process.Exe()
		if exePath == path {
			isRunning, err := process.IsRunning()
			if err != nil {
				return err
			}
			if isRunning {
				err = process.Kill()
				if err != nil {
					return err
				}
			}
			break
		}
	}
	klog.Infof("stop process:%v success", path)
	return nil
}
