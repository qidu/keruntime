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
	"encoding/json"
	"errors"

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
	if req.Method != http.MethodGet {
		msg := "only support get request method"
		util.ResponseError(http.StatusBadRequest, msg, w)
		return
	}
	query := req.URL.Query()
	appName := query.Get("appname")
	if appName == "" {
		msg := "request param must have appname"
		util.ResponseError(http.StatusBadRequest, msg, w)
		return
	}
	resource, err := message.BuildResource("", appsdconfig.Config.RegisterNodeNamespace, model.ResourceTypeConfigmap, appName)
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
	configMaps, err := formatConfigmapResp(data)
	if err != nil {
		util.ResponseError(http.StatusInternalServerError, err.Error(), w)
		return
	}

	util.ResponseSuccess(configMaps, w)
}

func (a *appsd) handleApp(msg *model.Message) {
	// todo: optimize code
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
