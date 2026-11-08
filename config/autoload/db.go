package autoload

type DB struct {
	Dsn              string `yaml:"dsn"`
	Driver           string `yaml:"driver"`
	DbName           string `yaml:"db_name"`
	Host             string `yaml:"host"`
	UserName         string `yaml:"username"`
	Password         string `yaml:"password"`
	Charset          string `yaml:"charset"`
	Port             int    `yaml:"port"`
	Prefix           string `yaml:"prefix"`
	MaxIdleConn      int    `yaml:"max_idle_conn"`
	MaxOpenConn      int    `yaml:"max_open_conn"`
	MaxLifeTime      int64  `yaml:"max_life_time"` // 连接最大寿命（秒），0 表示不限制
	ParseTime        string `yaml:"parse_time"`
	Loc              string `yaml:"loc"`
	MaxRetries       int    `yaml:"max_retries"`
	RetryDelay       int    `yaml:"retry_delay"`        // 外层连接重试间隔（秒）
	ConnectTimeoutMS int64  `yaml:"connect_timeout_ms"` // 单次初始化期限（毫秒），包含连接、握手和探测；0 使用 5 秒
}
