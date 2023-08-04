package util

import (
	"context"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/kubeedge/kubeedge/cloud/pkg/common/client"
	"github.com/kubeedge/kubeedge/cloud/pkg/edgecontroller/constants"
)

var (
	kubeClient kubernetes.Interface
)

func init() {
	kubeClient = client.GetKubeClient()
}

//return names of edge node in ready state
func GetEdgeNodes() ([]string, error) {
	nodeList, err := kubeClient.CoreV1().Nodes().List(context.Background(), metaV1.ListOptions{})
	if err != nil {
		klog.Warningf("query edge nodes failed:%v", err)
		return nil, err
	}
	var nodeNames []string
	for _, node := range nodeList.Items {
		if _, ok:= node.Labels[constants.EdgeNodeLabel]; !ok {
			continue
		}
		conditions := node.Status.Conditions
		nodeStatus := conditions[len(conditions)-1].Type
		if nodeStatus != constants.Ready {
			continue
		}
		nodeNames = append(nodeNames, node.Name)
	}
	return nodeNames, nil
}