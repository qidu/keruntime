package config

import (
	"sync"

	"github.com/kubeedge/kubeedge/pkg/apis/componentconfig/edgecore/v1alpha2"
)

var Config Configure
var once sync.Once

type Configure struct {
	v1alpha2.Appsd
}

func InitConfigure(a *v1alpha2.Appsd) {
	once.Do(func() {
		Config = Configure{
			Appsd: *a,
		}
	})
}

