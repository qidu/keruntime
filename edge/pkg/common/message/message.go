package message

import (
	"fmt"

	"github.com/kubeedge/beehive/pkg/core/model"
	"github.com/kubeedge/kubeedge/common/constants"
)

//constant defining node connection types
const (
	ResourceTypeNodeConnection = "node/connection"
	SourceNodeConnection       = "edgehub"
	OperationNodeConnection    = "connect"
	OperationSubscribe         = "subscribe"
	OperationUnsubscribe       = "unsubscribe"
	OperationMessage           = "message"
	OperationPublish           = "publish"
	OperationGetResult         = "get_result"
	OperationResponse          = "response"
	OperationKeepalive         = "keepalive"

	ResourceGroupName = "resource"
	TwinGroupName     = "twin"
	FuncGroupName     = "func"
	UserGroupName     = "user"

	ResourceNode = "node"
)

//BuildMsg returns message object with router and content details
func BuildMsg(group, parentID, sourceName, resource, operation string, content interface{}) *model.Message {
	msg := model.NewMessage(parentID).BuildRouter(sourceName, group, resource, operation).FillBody(content)
	return msg
}

// BuildResource return a string as "beehive/pkg/core/model".Message.Router.Resource
func BuildResource(nodeID, namespace, resourceType, resourceID, appName, domain string) (resource string, err error) {
	if namespace == "" || resourceType == "" {
		err = fmt.Errorf("required parameter are not set (namespace or resource type)")
		return "", err
	}

	if nodeID != "" {
		resource = fmt.Sprintf("%s%s%s", ResourceNode, constants.ResourceSep, nodeID)
	}

	if resource != "" {
		resource += fmt.Sprintf("%s%s%s%s", constants.ResourceSep, namespace, constants.ResourceSep, resourceType)
	} else {
		resource += fmt.Sprintf("%s%s%s", namespace, constants.ResourceSep, resourceType)
	}

	if resourceID != "" {
		resource += fmt.Sprintf("%s%s", constants.ResourceSep, resourceID)
	}

	if appName != "" {
		resource += fmt.Sprintf("%s%s", constants.ResourceSep, appName)
	}
	
	if domain != "" {
		resource += fmt.Sprintf("%s%s", constants.ResourceSep, domain)
	}

	return
}
