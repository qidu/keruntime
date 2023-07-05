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

@CHANGELOG
KubeEdge Authors: To create mini-kubelet for edge deployment scenario,
This file is derived from K8S Kubelet code with reduced set of methods
Changes done are
1. Package edged got some functions from "k8s.io/kubernetes/pkg/kubelet/kubelet.go"
and made some variant
*/

package appsd

import (
	"context"
	"encoding/json"
	"errors"

	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kubeedge/beehive/pkg/core"
	beehiveContext "github.com/kubeedge/beehive/pkg/core/context"
	model "github.com/kubeedge/beehive/pkg/core/model"
	appsdconfig "github.com/kubeedge/kubeedge/edge/pkg/appsd/config"
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

type serverResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Body interface{} `json:"body"`
}

var _ core.Module = (*appsd)(nil)
var lock sync.Mutex
var operationMap = make(map[string]bool)

// newAppsd creates new appsd object and initialises it
func newAppsd(enable bool) *appsd {
	return &appsd{
		enable: enable,
	}
}

// Register register edged
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
	mux.HandleFunc("/configmap", queryConfigMapHandler)

	// todo: support tls
	s := http.Server{
		Addr:    fmt.Sprintf("%s:%d", appsdconfig.Config.Server, appsdconfig.Config.Port),
		Handler: mux,
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
	utilruntime.HandleError(s.ListenAndServe())
}

func queryConfigMapHandler(w http.ResponseWriter, req *http.Request) {
	sResp := &serverResponse{}
	if req.Method != http.MethodGet {
		sResp.Code = http.StatusBadRequest
		sResp.Msg = "only support get request method"
		w.Write(marshalResult(sResp))
		return
	}
	query := req.URL.Query()
	appName := query.Get("appname")
	if appName == "" {
		sResp.Code = http.StatusBadRequest
		sResp.Msg = "request param must have appname"
		w.Write(marshalResult(sResp))
		return
	}

	msg := model.NewMessage("").BuildRouter(modules.AppsdModuleName, modules.AppsdGroup,
		appsdconfig.Config.RegisterNodeNamespace+"/"+model.ResourceTypeConfigmap+"/"+appName, model.QueryOperation)

	responseMessage, err := beehiveContext.SendSync(modules.MetaManagerModuleName, *msg, time.Second*10)
	if err != nil {
		sResp.Code = http.StatusBadRequest
		sResp.Msg = err.Error()
		w.Write(marshalResult(sResp))
		return
	}
	resp, err := responseMessage.GetContentData()
	if err != nil {
		sResp.Code = http.StatusInternalServerError
		sResp.Msg = err.Error()
		w.Write(marshalResult(sResp))
		return
	}

	var data []string
	err = json.Unmarshal(resp, &data)
	if err != nil {
		sResp.Code = http.StatusInternalServerError
		sResp.Msg = err.Error()
		w.Write(marshalResult(sResp))
		return
	}

	configMaps, err := formatConfigmapResp(data)
	if err != nil {
		sResp.Code = http.StatusInternalServerError
		sResp.Msg = err.Error()
		w.Write(marshalResult(sResp))
		return
	}
	sResp.Code = http.StatusOK
	sResp.Msg = "success"
	sResp.Body = configMaps
	w.Write(marshalResult(sResp))
}

func (a *appsd) handleApp(msg *model.Message) {
	// todo: optimize code
	return
}

func marshalResult(sResp *serverResponse) (resp []byte) {
	resp, _ = json.Marshal(sResp)
	return
}

func formatConfigmapResp(data []string) ([]map[string]string, error) {
	if data == nil || len(data) == 0 {
		return nil, errors.New("data is empty")
	}
	configMaps := []map[string]string{}
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
