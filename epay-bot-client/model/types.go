package model

import "time"

// EpayResponse represents the standard response from the epay API
type EpayResponse[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data []T    `json:"data"`
}

// Order represents an order from the epay API
type Order struct {
	TradeNo string `json:"trade_no"`
	OutTradeNo string `json:"out_trade_no"`
	Type       string `json:"type"`
	Pid        string `json:"pid"`
	Addtime    string `json:"addtime"`
	Endtime    string `json:"endtime"`
	Name       string `json:"name"`
	Money      string `json:"money"`
	Status     interface{} `json:"status"` // Can be int or string in some APIs, safer to parse later or use string
}

// Settlement represents a settlement from the epay API
type Settlement struct {
	ID        string `json:"id"`
	Pid       string `json:"pid"`
	Type      string `json:"type"`
	Account   string `json:"account"`
	Money     string `json:"money"`
	Realmoney string `json:"realmoney"`
	Addtime   string `json:"addtime"`
	Endtime   string `json:"endtime"`
	Status    interface{} `json:"status"`
}

// MerchantInfo represents the merchant configuration for a user
type MerchantInfo struct {
	ChatID int64
	Domain string
	Pid    string
	Key    string
}

// PollingStatus represents the polling status for a user
type PollingStatus struct {
	ChatID   int64
	Active   bool
	LastPoll time.Time
}
