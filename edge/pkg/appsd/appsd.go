/*
Copyright 2016 The Kubernetes Authors.

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

package appsd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	commonType "github.com/kubeedge/kubeedge/common/types"
	"strings"
	"sync"

	"fmt"
	"net/http"
	"time"

	"github.com/kubeedge/beehive/pkg/core"
	beehiveContext "github.com/kubeedge/beehive/pkg/core/context"
	model "github.com/kubeedge/beehive/pkg/core/model"
	appsdconfig "github.com/kubeedge/kubeedge/edge/pkg/appsd/config"
	"github.com/kubeedge/kubeedge/edge/pkg/appsd/util"
	"github.com/kubeedge/kubeedge/edge/pkg/common/message"
	"github.com/kubeedge/kubeedge/edge/pkg/common/modules"
	"github.com/kubeedge/kubeedge/pkg/apis/componentconfig/edgecore/v1alpha2"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
)

// appsd is the main appsd implementation.
type appsd struct {
	enable bool
}

var (
	_ core.Module = (*appsd)(nil)
	operationMap sync.Map
)

// newAppsd creates new appsd object and initialises it
func newAppsd(enable bool) *appsd {
	return &appsd{
		enable: enable,
	}
}

// Register register appsd
func Register(a *v1alpha2.Appsd) {
	appsdconfig.InitConfigure(a)
	appsd := newAppsd(a.Enable)
	core.Register(appsd)
}

func (a *appsd) Name() string {
	return modules.AppsdModuleName
}

func (a *appsd) Group() string {
	return modules.AppsdGroup
}

// Enable indicates whether this module is enabled
func (e *appsd) Enable() bool {
	return e.enable
}

func (a *appsd) Start() {
	klog.Info("Starting appsd...")

	go server(beehiveContext.Done())

	for {
		select {
		case <-beehiveContext.Done():
			klog.Warning("appsd stop")
			return
		default:
		}
		msg, err := beehiveContext.Receive(modules.AppsdModuleName)
		if err != nil {
			klog.Warningf("appsd receive msg error %v", err)
			continue
		}
		klog.V(4).Info("appsd receive msg")
		go a.handleApp(&msg)
	}
}

func server(stopChan <-chan struct{}) {
	mux := http.NewServeMux()
	mux.HandleFunc("/config", queryConfigHandler)

	certificate, err := util.CreateCertificate()
	if err != nil {
		klog.Errorf("create cert failed: %v", certificate)
		return
	}
	config := &tls.Config{
		Certificates: []tls.Certificate{*certificate},
		MinVersion:   tls.VersionTLS12,
	}
  
  s := http.Server{
		Addr:    fmt.Sprintf("%s:%d", appsdconfig.Config.Server, appsdconfig.Config.Port),
		Handler: mux,
		TLSConfig: config,
	}

	go func() {
		<-stopChan

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			klog.Errorf("Server shutdown failed: %s", err)
		}
	}()

	klog.Infof("[appsdserver]start to listen and server at http://%v", s.Addr)
	utilruntime.HandleError(s.ListenAndServeTLS("",""))
}

func queryConfigHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		msg := "only support get request method"
		util.ResponseError(http.StatusBadRequest, msg, w)
		return
	}
	query := req.URL.Query()
	appName := query.Get("appname")
	configType := query.Get("type")
	if appName == "" || configType == "" {
		msg := "request param must have appname and type"
		util.ResponseError(http.StatusBadRequest, msg, w)
		return
	}
	resource, err := message.BuildResource("", appsdconfig.Config.RegisterNodeNamespace, configType, appName)
	msg := model.NewMessage("").BuildRouter(modules.AppsdModuleName, modules.AppsdGroup, resource, model.QueryOperation)
	responseMessage, err := beehiveContext.SendSync(modules.MetaManagerModuleName, *msg, time.Second*10)
	if err != nil {
		util.ResponseError(http.StatusBadRequest, err.Error(), w)
		return
	}
	resp, err := responseMessage.GetContentData()
	if err != nil {
		util.ResponseError(http.StatusInternalServerError, err.Error(), w)
		return
	}

	var data []string
	err = json.Unmarshal(resp, &data)
	if err != nil {
		util.ResponseError(http.StatusInternalServerError, err.Error(), w)
		return
	}

	var respData interface{}
	switch configType {
	case model.ResourceTypeConfigmap:
		respData, err = formatConfigmapResp(data)
	case model.ResourceTypeSecret:
		respData, err = formatSecretResp(data)
	default:
		klog.Errorf("configType is not configmap or secret: configType is %s", configType)
	}
	if err != nil {
		util.ResponseError(http.StatusInternalServerError, err.Error(), w)
		return
	}

	util.ResponseSuccess(respData, w)
}

func (a *appsd) handleApp(msg *model.Message) {
	// todo: optimize code
	resource := msg.GetResource()
	r := strings.Split(resource, "/")
	if len(r) != 2 {
		m := "the format of resource " + resource + " is incorrect"
		klog.Warningf(m)
		return
	}
	content, err := msg.GetContentData()
	if err != nil {
		klog.Errorf("get message content data failed: %v", err)
		return
	}

	var args []string
	if err := json.Unmarshal(content, &args); err != nil {
		m := "error to parse app command"
		klog.Errorf(m)
		return
	}

	var appCommand commonType.AppCommand
	appCommand.Action = msg.GetOperation()
	appCommand.Path = args[0]
	if len(args) > 1 {
		appCommand.Args = args[1]
	}

	operationKey := appCommand.Path + appCommand.Args

	switch msg.GetOperation() {
	case model.InsertOperation:
		processMsg(operationKey, func() {
			err = a.startApp(appCommand)
			if err != nil {
				operationMap.Delete(operationKey)
				klog.Errorf("start app failed:%v", err)
			}
		})
	case model.DeleteOperation:
		err = a.StopApp(appCommand)
		if err != nil {
			klog.Errorf("delete app failed:%v", err)
		}
		if _, ok := operationMap.Load(operationKey); ok {
			operationMap.Delete(operationKey)
			return
		}
	case model.UpdateOperation:
		processMsg(operationKey, func() {
			err = a.updateApp(appCommand)
			if err != nil {
				operationMap.Delete(operationKey)
				klog.Errorf("start app failed:%v", err)
			}
		})
	default:
		klog.Errorf("unsupport app operation:%v", msg.GetOperation())
	}
	return
}

func (a *appsd) startApp(appCommand commonType.AppCommand) error {
	err := util.StartProcess(appCommand.Path, appCommand.Args)
	if err != nil {
		return err
	}
	return nil
}

func (a *appsd) StopApp(appCommand commonType.AppCommand) error {
	err := util.StopProcess(appCommand.Path)
	if err != nil {
		return err
	}
	return nil
}

func (a *appsd) updateApp(appCommand commonType.AppCommand) error {
	//todo: add code
	err := util.StartProcess(appCommand.Path, appCommand.Args)
	if err != nil {
		return err
	}
	return nil
}

func processMsg(operationKey string, operationFunc func()) {
	if _, ok := operationMap.Load(operationKey); ok {
		return
	}
	defer operationMap.Delete(operationKey)
	operationMap.Store(operationKey, true)

	operationFunc()
	// wait for all duplicate messages to disappear and avoid duplicate execution
	time.Sleep(5 * time.Second)
}

func formatConfigmapResp(data []string) ([]map[string]string, error) {
	if data == nil || len(data) == 0 {
		return nil, errors.New("data is empty")
	}
	var configMaps []map[string]string
	for _, v := range data {
		cm := new(v1.ConfigMap)
		err := json.Unmarshal([]byte(v), cm)
		if err != nil {
			return nil, err
		}
		configMaps = append(configMaps, (*cm).Data)
	}
	return configMaps, nil
}

func formatSecretResp(data []string) ([]map[string][]byte, error) {
	if data == nil || len(data) == 0 {
		return nil, errors.New("data is empty")
	}
	var secrets []map[string][]byte
	for _, v := range data {
		s := new(v1.Secret)
		err := json.Unmarshal([]byte(v), s)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, (*s).Data)
	}
	return secrets, nil
}