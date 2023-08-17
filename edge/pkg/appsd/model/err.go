package model

import "fmt"

type Error struct  {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"msg"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("error:status=%v code=%s, message=%s", e.Status, e.Code, e.Message)
}

func New(status int, code, message string) *Error {
	return &Error{
		Status:  status,
		Code:    code,
		Message: message,
	}
}

var (
	Success             = New(200, "1000", "Success")
	ErrInvalidParam     = New(400, "1002", "Invalid parameter")
	ErrCertEmpty        = New(403, "1112", "Domain has no cert")
	ErrRequestMethod    = New(405, "1113", "Request method error")
	ErrInternalServer   = New(500, "1001", "Internal server error")
	ErrJsonUnmarshal    = New(500, "1107", "Json unmarshal error")
	ErrFormatResponse   = New(500, "1108", "Format http response error")
)