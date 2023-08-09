package util

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"k8s.io/klog/v2"
)

const (
	processShutdownTimeout = 15
)

// Native app start command path and arguments
type AppCommand struct {
	Action string
	Path   string       // absolute executable command path
	Args   []string     // native app args
	Envs   []string     // environment variables
}

func CheckCmdExists(cmd string) (bool, error) {
	isAbs := filepath.IsAbs(cmd)
	if !isAbs {
		err := errors.New("executable command must be absolute path")
		return false, err
	}
	_, err := exec.LookPath(cmd)
	if err != nil {
		klog.Errorf("cannot find command:%s\n", cmd)
		return false, err
	}
	return true, nil
}

func StartProcess(command AppCommand) error {
	var err error
	if ok, err := CheckCmdExists(command.Path); !ok {
		return err
	}
	envs := os.Environ()
	envs = append(envs, command.Envs...)

	cmd := exec.Command(command.Path, command.Args...)
	cmd.Env = envs

	var stdin, stdout, stderr bytes.Buffer
	cmd.Stdin = &stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return err
	}
	outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	klog.Infof("exec command:%s,%v\n,out:%s\nerr:%s", command.Path, command.Args, outStr, errStr)
	return nil
}

func StopProcess(command AppCommand) error {
	path := command.Path
	targetProcess, err := FindProcess(path)
	if err != nil {
		return err
	}
	if targetProcess == nil {
		return fmt.Errorf("path %s is not exist", path)
	}

	var isRunning bool
	retry := 3
Loop:
	for retry > 0 {
		isRunning, _ = targetProcess.IsRunning()
		if !isRunning {
			break
		}
		err = syscall.Kill(int(targetProcess.Pid), syscall.SIGTERM)
		if err != nil {
			return err
		}
		// Wait up to 15secs for it to stop
		for i := time.Duration(0); i < processShutdownTimeout; i += time.Second {
			isRunning, _ = targetProcess.IsRunning()
			if !isRunning {
				break Loop
			}
			time.Sleep(time.Second)
		}
		retry--
	}
	if isRunning {
		err = syscall.Kill(int(targetProcess.Pid), syscall.SIGKILL)
		if err != nil {
			return err
		}
	}
	klog.Infof("stop process:%v success", path)
	return nil
}

//find process that match the absolute executable command path
func FindProcess(path string) (*process.Process, error) {
	isAbs := filepath.IsAbs(path)
	if !isAbs {
		err := errors.New("executable command must be absolute path")
		return nil, err
	}
	var targetProcess *process.Process 
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}
	for _, p := range processes {
		exePath, _ := p.Exe()
		if exePath == path {
			targetProcess = p
			break
		}
	}
	return targetProcess, nil
}

func GenerateCommand(args []string) *AppCommand {
	if args == nil || len(args) == 0 {
		return nil
	}
	appCommand := &AppCommand{}
	pathIndex := -1
	for index, arg := range args {
		if ok, _ := CheckCmdExists(arg); ok {
			appCommand.Path = arg
			pathIndex = index
			break
		}
	}
	var envs []string
	for i := 0; i < pathIndex; i++ {
		envs = append(envs, args[i])
	}
	var arguments []string
	for i := pathIndex+1; i < len(args); i++ {
		arguments = append(arguments, args[i])
	}
	appCommand.Args = arguments
	appCommand.Envs = envs
	return appCommand
}