package util

import (
	"encoding/json"
	"net/http"
)

const (
	SUCCESS = "success"
)

type serverResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Body interface{} `json:"body"`
}

func ResponseError(code int, msg string, w http.ResponseWriter) {
	resp := serverResponse{
		Code: code,
		Msg:  msg,
		Body: nil,
	}
	w.Write(marshalResult(&resp))
}

func ResponseSuccess(data interface{}, w http.ResponseWriter) {
	resp := serverResponse{
		Code: http.StatusOK,
		Msg:  SUCCESS,
		Body: data,
	}
	w.Write(marshalResult(&resp))
}

func marshalResult(sResp *serverResponse) (resp []byte) {
	resp, _ = json.Marshal(sResp)
	return
}

