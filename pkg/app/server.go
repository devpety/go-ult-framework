// Package app 提供应用管理功能。
package app

import "github.com/devpety/go-utils/v3/server"

// ServerAgreement 服务器协议信息。
type ServerAgreement struct {
	Network string // 配置的传输网络（tcp 等）
	Addr    string // 有效 Endpoint 地址（host:port）
	Target  string // 公布的访问 URL，包含 HTTP/HTTPS scheme
}

// Server 服务器接口，扩展 go-utils Server 接口。
type Server interface {
	server.Server

	ServerType() string // 获取服务器类型描述
	StartBefore()       // 启动前钩子
	StartAfter()        // 启动后钩子
	CancelBefore()      // 关闭前钩子
	CancelAfter()       // 关闭后钩子
}
