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
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/abrander/go-supervisord"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"

	"github.com/kubeedge/beehive/pkg/core"
	beehiveContext "github.com/kubeedge/beehive/pkg/core/context"
	model "github.com/kubeedge/beehive/pkg/core/model"
	"github.com/kubeedge/kubeedge/common/constants"
	appsdconfig "github.com/kubeedge/kubeedge/edge/pkg/appsd/config"
	edgedconfig "github.com/kubeedge/kubeedge/edge/pkg/edged/config"
	"github.com/kubeedge/kubeedge/edge/pkg/appsd/util"
	appsdmodel "github.com/kubeedge/kubeedge/edge/pkg/appsd/model"
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
	supervisordClient *supervisord.Client
)

// newAppsd creates new appsd object and initialises it
func newAppsd(enable bool) *appsd {
	var err error
	supervisordClient, err = supervisord.NewUnixSocketClient(appsdconfig.Config.SupervisordEndpoint)
	if err != nil {
		klog.Exitf("new supervisord client failed with error: %s", err)
	}
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
		util.ResponseError(w, msg, appsdmodel.ErrRequestMethod)
		return
	}
	query := req.URL.Query()
	appName := query.Get("appname")
	configType := query.Get("type")
	domain := query.Get("domain")
	if configType == "" {
		msg := "request param must have type field, the value is configmap or secert"
		util.ResponseError(w, msg, appsdmodel.ErrInvalidParam)
		return
	}
	if domain == "" && appName == "" {
		msg := "request param must have appname, when query domain cert also includes domain"
		util.ResponseError(w, msg, appsdmodel.ErrInvalidParam)
		return
	}
	responseMessage, err := queryConfigFromMetaManager(configType, appName, domain)
	if err != nil {
		util.ResponseError(w, err.Error(), appsdmodel.ErrInternalServer)
		return
	}
	resp, err := responseMessage.GetContentData()
	if err != nil {
		util.ResponseError(w, err.Error(), appsdmodel.ErrInternalServer)
		return
	}

	var data []string
	err = json.Unmarshal(resp, &data)
	if err != nil {
		util.ResponseError(w, err.Error(), appsdmodel.ErrJsonUnmarshal)
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
		util.ResponseError(w, err.Error(), appsdmodel.ErrFormatResponse)
		return
	}

	util.ResponseSuccess(w, respData)
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

	customUuid := ""
	nativeApp := ""
	if pod.Labels != nil {
		uuid, ok := pod.Labels["uuid"]
		if ok {
			customUuid = uuid
		}
		appName, ok := pod.Labels["appName"]
		if ok {
			nativeApp = appName
		}
	}
	if customUuid == "" || nativeApp == "" {
		m := "must specify uuid and appName in pod labels"
		klog.Error(m)
		return
	}

	operationKey := fmt.Sprintf("%s:%s:%s", pod.Namespace, 
		pod.Name, nativeApp)
	switch msg.GetOperation() {
	case model.InsertOperation:
		processMsg(operationKey, customUuid, func() {
			err = a.startApp(nativeApp)
			if err != nil {
				operationMap.Delete(operationKey)
				klog.Errorf("start app failed:%v", err)
			}
		})
	case model.DeleteOperation:
		err = a.StopApp(nativeApp)
		if err != nil {
			klog.Errorf("delete app failed:%v", err)
			return
		}
		if _, ok := operationMap.Load(operationKey); ok {
			operationMap.Delete(operationKey)
		}
	case model.UpdateOperation:
		processMsg(operationKey, customUuid, func() {
			err = a.updateApp(nativeApp)
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

func (a *appsd) startApp(appName string) error {
	err := supervisordClient.StartProcess(appName, false)
	if err != nil {
		return err
	}
	return nil
}

func (a *appsd) StopApp(appName string) error {
	err := supervisordClient.StopProcess(appName, false)
	if err != nil {
		return err
	}
	return nil
}

func (a *appsd) updateApp(appName string) error {
	//query native app supervisor config in configmap from metamanager
	supervisorConfig, err := getNativeAppConfig(appName, constants.DefaultSupervisorConfKey)
	if err != nil {
		klog.Errorf("get app config failed: %v", err)
		return err 
	}
	appConfigPath := fmt.Sprintf("%s/%s.conf", appsdconfig.Config.SupervisordConfDir, appName)
	// check local config file
	isExist, err := util.CheckFileExists(appConfigPath)
	if err != nil {
		klog.Error(err)
		return err
	}
	if supervisorConfig == "" && !isExist {
		err = fmt.Errorf("cannot find config for %s", appName)
		klog.Error(err)
		return err 
	}
	if isExist {
		content, err := os.ReadFile(appConfigPath)
		if err != nil {
			klog.Errorf("read config file %s failed: %v", appConfigPath, err)
			return err 
		}
		//check if there are any changes in the service supervisor config of the configmap
		ok := util.ValidateFileContent(string(content), supervisorConfig)
		//local config file exist, but the config in configmap does not exist or not updated
		if ok || supervisorConfig == "" {
			processInfo, err := supervisordClient.GetProcessInfo(appName)
			if err != nil {
				klog.Errorf("get %s process info failed: %v", appName, err)
				return err
			}
			if processInfo.StateName == constants.SupervisorServiceRunning {
				err = supervisordClient.StopProcess(appName, true)
				if err != nil {
					klog.Errorf("stop process %v failed: %v", appName, err)
					return err 
				}
			}
			err = supervisordClient.StartProcess(appName, false)
			if err != nil {
				klog.Errorf("start process %v failed: %v", appName, err)
				return err 
			}
		} else {
			appConfigBakPath := fmt.Sprintf("%s.%d", appConfigPath, time.Now().Unix())
			//backup old config file
			err = util.RenameFile(appConfigPath, appConfigBakPath)
			if err != nil {
				klog.Errorf("rename config file %s to %s failed: %v", appConfigPath, appConfigBakPath)
				return err 
			}
			//generate new config file by config in configmap
			err = util.CreateFile(appConfigPath, supervisorConfig)
			if err != nil {
				klog.Errorf("create config file %s failed: %v", appConfigPath)
				return err 
			}
			//reload config file, start app
			err = supervisordClient.Update()
			if err != nil {
				klog.Errorf("supervisord reload config file %s failed: %v", appConfigPath)
				return err
			}
		}
	} else {
		err = util.CreateFile(appConfigPath, supervisorConfig)
		if err != nil {
			klog.Errorf("create config file %s failed: %v", appConfigPath)
			return err 
		}
		err = supervisordClient.StartProcess(appName, false)
		if err != nil {
			klog.Errorf("start process %v failed: %v", appName, err)
			return err 
		}
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

func getNativeAppConfig(appName, configKey string) (string, error) {
	responseMessage, err := queryConfigFromMetaManager(model.ResourceTypeConfigmap, appName, "")
	if err != nil {
		klog.Errorf("query config from meta manager failed: %v", err)
		return "", err
	}
	resp, err := (*responseMessage).GetContentData()
	if err != nil {
		klog.Errorf("get response message content data failed: %v", err)
		return "", err	
	}
	var data []string
	err = json.Unmarshal(resp, &data)
	if err != nil {
		klog.Errorf("unmarshal data failed: %v", err)
		return "", err
	}
	// It is also allowed that the app config is not created by using configmap, just use local config
	if data == nil || len(data) == 0 {
		klog.Warning("the native app config is not created by using configmap, will use local config")
		return "", nil
	}
	appConfigs, err := formatConfigmapResp(data)
	if err != nil {
		klog.Errorf("format configmap resp failed: %v", err)
		return "", err
	}
	configItem, ok := appConfigs[configKey]
	if !ok {
		klog.Warning("the native app supervisor config is not created by using configmap, will use local config")
		return "", nil
	}
	return configItem, nil
}

func queryConfigFromMetaManager(resourceType, appName, domain string) (*model.Message, error) {
	resource, err := message.BuildResource(edgedconfig.Config.HostnameOverride,
		appsdconfig.Config.RegisterNodeNamespace, resourceType, appName, domain)
	msg := model.NewMessage("").BuildRouter(modules.AppsdModuleName,
			modules.AppsdGroup, resource, model.QueryOperation)
	responseMessage, err := beehiveContext.SendSync(modules.MetaManagerModuleName, *msg, time.Second*10)
	if err != nil {
		return nil, err
	}
	return &responseMessage, nil
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