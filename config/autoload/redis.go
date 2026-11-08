package autoload

// Redis Redis 连接配置结构体。
type Redis struct {
	Network            string `yaml:"network"`              // 网络协议 (tcp)
	Addr               string `yaml:"addr"`                 // Redis 地址
	Port               int    `yaml:"port"`                 // Redis 端口
	Username           string `yaml:"username"`             // 用户名
	Password           string `yaml:"password"`             // 密码
	DB                 int    `yaml:"db"`                   // 数据库索引
	StartupMaxAttempts int    `yaml:"startup_max_attempts"` // 初始化最大尝试次数，包含首次尝试；0 使用 3 次
	StartupRetryDelay  int    `yaml:"startup_retry_delay"`  // 初始化重试间隔（秒）；0 使用 1 秒
	MaxRetries         int    `yaml:"max_retries"`          // 命令最大重试次数；0 或 -1 禁用，正数仅用于可安全重放命令的连接
	ConnectTimeoutMS   int64  `yaml:"connect_timeout_ms"`   // 单次初始化期限（毫秒），包含连接、认证和探测；0 使用 5 秒
	MinRetryBackoff    int64  `yaml:"min_retry_backoff"`    // 毫秒；0 使用默认 8ms，-1 禁用
	MaxRetryBackoff    int64  `yaml:"max_retry_backoff"`    // 毫秒；0 使用默认 512ms，-1 禁用
	DialTimeout        int64  `yaml:"dial_timeout"`         // 毫秒；0 使用底层默认值
	ReadTimeout        int64  `yaml:"read_timeout"`         // 毫秒；0 使用默认值，-1 仅禁用默认 socket 超时，仍遵守上下文 Deadline
	WriteTimeout       int64  `yaml:"write_timeout"`        // 毫秒；0 使用默认值，-1 仅禁用默认 socket 超时，仍遵守上下文 Deadline
	PoolFIFO           bool   `yaml:"pool_fifo"`            // 连接池 FIFO 模式
	PoolSize           int    `yaml:"pool_size"`            // 连接池大小
	MinIdleConns       int    `yaml:"min_idle_conns"`       // 最小空闲连接数
	MaxConnAge         int64  `yaml:"max_conn_age"`         // 秒；0 或 -1 表示不限制连接寿命
	PoolTimeout        int64  `yaml:"pool_timeout"`         // 毫秒；0 使用底层默认值
	IdleTimeout        int64  `yaml:"idle_timeout"`         // 秒；0 使用默认 30 分钟，-1 禁用空闲过期检查
}
