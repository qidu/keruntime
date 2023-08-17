package util

import (
	"encoding/json"
	"net/http"

	appsdmodel "github.com/kubeedge/kubeedge/edge/pkg/appsd/model"
)

type serverResponse struct {
	Code     string      `json:"code"`
	Message  string      `json:"message"`
	Body interface{}     `json:"body"`
}

func ResponseError(w http.ResponseWriter, msg string, err *appsdmodel.Error) {
	resp := serverResponse{
		Code: 	  err.Code,
		Message:  msg,
		Body:     nil,
	}
	w.WriteHeader(err.Status)
	w.Write(marshalResult(&resp))
}

func ResponseSuccess(w http.ResponseWriter, data interface{}) {
	resp := serverResponse{
		Code:    appsdmodel.Success.Code,
		Message: appsdmodel.Success.Message,
		Body:    data,
	}
	w.WriteHeader(http.StatusOK)
	w.Write(marshalResult(&resp))
}

func marshalResult(sResp *serverResponse) (resp []byte) {
	resp, _ = json.Marshal(sResp)
	return
}

