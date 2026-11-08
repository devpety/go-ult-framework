package autoload

type JWT struct {
	App            string   `yaml:"app"`
	Issuer         string   `yaml:"issuer"`
	Algorithm      string   `yaml:"algorithm"`        // 留空时使用 HS256
	Secret         string   `yaml:"secret"`           // HMAC 密钥
	PrivateKeyFile string   `yaml:"private_key_file"` // 非对称签名私钥 PEM 文件，与公钥文件二选一
	PublicKeyFile  string   `yaml:"public_key_file"`  // 仅验签时使用的公钥 PEM 文件
	Audience       []string `yaml:"audience"`         // 可选接收方约束，任一匹配即可
}
