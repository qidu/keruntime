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
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"fmt"
	"net/http"
	"time"

	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"

	"github.com/kubeedge/beehive/pkg/core"
	beehiveContext "github.com/kubeedge/beehive/pkg/core/context"
	model "github.com/kubeedge/beehive/pkg/core/model"
	appsdconfig "github.com/kubeedge/kubeedge/edge/pkg/appsd/config"
	edgedconfig "github.com/kubeedge/kubeedge/edge/pkg/edged/config"
	"github.com/kubeedge/kubeedge/edge/pkg/appsd/util"
	"github.com/kubeedge/kubeedge/edge/pkg/common/message"
	"github.com/kubeedge/kubeedge/edge/pkg/common/modules"
	"github.com/kubeedge/kubeedge/pkg/apis/componentconfig/edgecore/v1alpha2"
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
	domain := query.Get("domain")
	if configType == "" {
		msg := "request param must have type field, the value is configmap or secert"
		util.ResponseError(http.StatusBadRequest, msg, w)
		return
	}
	if domain == "" && appName == "" {
		msg := "request param must have appname, when query domain cert also includes domain"
		util.ResponseError(http.StatusBadRequest, msg, w)
		return
	}
	resource, err := message.BuildResource(edgedconfig.Config.HostnameOverride,
		appsdconfig.Config.RegisterNodeNamespace, configType, appName, domain)
	msg := model.NewMessage("").BuildRouter(modules.AppsdModuleName,
			modules.AppsdGroup, resource, model.QueryOperation)
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
	
	var pod v1.Pod
	if err := json.Unmarshal(content, &pod); err != nil {
		m := "failed to parse pod"
		klog.Error(m)
		return
	}

	var args []string
	if pod.Spec.Containers[0].Args != nil && len(pod.Spec.Containers[0].Args) > 0 {
		args = pod.Spec.Containers[0].Args
	}
	customUuid := ""
	if pod.Labels != nil {
		uuid, ok := pod.Labels["uuid"]
		if ok {
			customUuid = uuid
		}
	}
	appCommand := util.GenerateCommand(args)
	operationKey := fmt.Sprintf("%s:%s:%s", pod.Namespace, 
		pod.Name, appCommand.Path)

	switch msg.GetOperation() {
	case model.InsertOperation:
		processMsg(operationKey, customUuid, func() {
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
			return
		}
		if _, ok := operationMap.Load(operationKey); ok {
			operationMap.Delete(operationKey)
		}
	case model.UpdateOperation:
		processMsg(operationKey, customUuid, func() {
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

func (a *appsd) startApp(appCommand util.AppCommand) error {
	err := util.StartProcess(appCommand)
	if err != nil {
		return err
	}
	return nil
}

func (a *appsd) StopApp(appCommand util.AppCommand) error {
	err := util.StopProcess(appCommand)
	if err != nil {
		return err
	}
	return nil
}

func (a *appsd) updateApp(appCommand util.AppCommand) error {
	targetProcess, err := util.FindProcess(appCommand.Path)
	if err != nil {
		return err
	}
	if targetProcess != nil {
		err = util.StopProcess(appCommand)
		if err != nil {
			klog.Errorf("stop process %s failed:%v", appCommand.Path, err)
			return err
		} 
	}
	err = util.StartProcess(appCommand)
	if err != nil {
		return err
	}
	return nil
}

func processMsg(operationKey, newUuid string, operationFunc func()) {
	oldUuid, ok := operationMap.Load(operationKey); 
	if ok && (newUuid == oldUuid) {
		return 
	}
	operationMap.Store(operationKey, newUuid)
	operationFunc()
}

func formatConfigmapResp(data []string) (map[string]string, error) {
	if data == nil || len(data) == 0 {
		return nil, errors.New("data is empty")
	}
	cm := new(v1.ConfigMap)
	err := json.Unmarshal([]byte(data[0]), cm)
	if err != nil {
		return nil, err
	}
	res := map[string]string{}
	if cm.Data != nil && len(cm.Data) > 0 {
		return cm.Data, nil
	} else {
		for k, v := range cm.BinaryData {
			res[k] = string(v)
		}
	}
	return res, nil
}

func formatSecretResp(data []string) ([]map[string]string, error) {
	if data == nil || len(data) == 0 {
		return nil, errors.New("data is empty")
	}
	var res []map[string]string
	for _, v := range data {
		s := new(v1.Secret)
		err := json.Unmarshal([]byte(v), s)
		if err != nil {
			return nil, err
		}
		singleSecretData := map[string]string{}
		if s.Data != nil && len(s.Data) > 0 {
			for k, v := range s.Data {
				bytes, err := base64.StdEncoding.DecodeString(string(v))
				if err == nil {
					singleSecretData[k] = string(bytes)
				} else {
					singleSecretData[k] = string(v)
				}
			}
		} else {
			for k, v := range s.StringData {
				bytes, err := base64.StdEncoding.DecodeString(v)
				if err == nil {
					singleSecretData[k] = string(bytes)
				} else {
					singleSecretData[k] = v
				}
			}
		}
		res = append(res, singleSecretData)
	}
	return res, nil
}