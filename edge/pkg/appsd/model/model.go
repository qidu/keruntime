package model

type DomainCertResponse struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Cert       string `json:"cert"`
	Key        string `json:"key"`
	TTL        string `json:"ttl"`
	ExpireAt   string `json:"expireAt"`
}