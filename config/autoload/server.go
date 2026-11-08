// Package autoload 提供配置模块自动加载。
package autoload

// Server 服务器配置结构体。
type Server struct {
	Http Http // HTTP 服务器配置
}

// Http HTTP 服务器配置结构体。
// 定义 HTTP 服务器的网络、地址、端口和 CORS 配置。
type Http struct {
	Network             string `yaml:"network"`                // 网络协议 (tcp)
	Host                string `yaml:"host"`                   // 服务主机地址
	Port                int    `yaml:"port"`                   // 服务端口
	MaxBodyBytes        int64  `yaml:"max_body_bytes"`         // 字节；0 使用默认 2 MiB
	ReadHeaderTimeoutMS int64  `yaml:"read_header_timeout_ms"` // 毫秒；0 使用默认 5 秒
	ReadTimeoutMS       int64  `yaml:"read_timeout_ms"`        // 毫秒；0 使用默认 15 秒
	WriteTimeoutMS      int64  `yaml:"write_timeout_ms"`       // 毫秒；0 使用默认 30 秒
	IdleTimeoutMS       int64  `yaml:"idle_timeout_ms"`        // 毫秒；0 使用默认 60 秒
	Cors                CORS   `yaml:"cors"`
}

// CORS 明确声明跨域来源、请求头和凭证策略；默认关闭跨域访问。
type CORS struct {
	Enabled          bool     `yaml:"enabled"`
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
}
