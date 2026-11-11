// Package auth 提供认证相关的基础设施构造。
package auth

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"ult/config"
	"ult/config/autoload"

	utilsauth "github.com/devpety/go-utils/v3/auth"
)

// NewJWT 按配置选择唯一算法，非对称密钥文件的相对路径以配置目录为基准。
func NewJWT(conf *config.Config) (utilsauth.JWT, error) {
	if conf == nil {
		return nil, fmt.Errorf("JWT 配置不能为空")
	}
	algorithm := utilsauth.JWTAlgorithm(conf.JWT.Algorithm)
	if algorithm == "" {
		algorithm = utilsauth.HS256
	}
	key, err := jwtKey(conf.JWT, algorithm, conf.ProjectPath)
	if err != nil {
		return nil, err
	}
	return utilsauth.NewJWT(conf.JWT.App, conf.JWT.Issuer, key,
		utilsauth.WithJWTAlgorithm(algorithm), utilsauth.WithJWTAudience(conf.JWT.Audience...))
}

// jwtKey 根据配置选择密钥。
// 支持对称和非对称算法，根据算法选择不同的密钥来源。
func jwtKey(conf autoload.JWT, algorithm utilsauth.JWTAlgorithm, basePath string) (any, error) {
	switch algorithm {
	case utilsauth.HS256, utilsauth.HS384, utilsauth.HS512:
		if conf.PrivateKeyFile != "" || conf.PublicKeyFile != "" {
			return nil, fmt.Errorf("JWT %s 只能配置 secret，不能配置非对称密钥文件", algorithm)
		}
		return conf.Secret, nil
	case utilsauth.RS256, utilsauth.RS384, utilsauth.RS512, utilsauth.PS256, utilsauth.PS384, utilsauth.PS512,
		utilsauth.ES256, utilsauth.ES384, utilsauth.ES512, utilsauth.EdDSA:
		if conf.Secret != "" || (conf.PrivateKeyFile == "") == (conf.PublicKeyFile == "") {
			return nil, fmt.Errorf("JWT %s 必须且只能配置 private_key_file 或 public_key_file，不能配置 secret", algorithm)
		}
	default:
		return nil, fmt.Errorf("%w: %q", utilsauth.ErrJWTAlgorithm, algorithm)
	}

	private := conf.PrivateKeyFile != ""
	filename := conf.PublicKeyFile
	if private {
		filename = conf.PrivateKeyFile
	}
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(basePath, filename)
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取 JWT 密钥文件失败: %w", err)
	}
	key, err := parseJWTKey(contents, private)
	if err != nil {
		return nil, fmt.Errorf("解析 JWT 密钥文件 %s 失败: %w", filename, err)
	}
	return key, nil
}

// 每个文件只接受一个未加密密钥块，避免多个密钥或私钥/公钥字段混用。
func parseJWTKey(contents []byte, private bool) (any, error) {
	contents = bytes.TrimSpace(contents)
	block, rest := pem.Decode(contents)
	if block == nil || !bytes.HasPrefix(contents, []byte("-----BEGIN ")) || len(bytes.TrimSpace(rest)) > 0 || len(block.Headers) > 0 {
		return nil, fmt.Errorf("必须提供单个未加密的 PEM 密钥块")
	}
	if private {
		switch block.Type {
		case "PRIVATE KEY":
			return x509.ParsePKCS8PrivateKey(block.Bytes)
		case "RSA PRIVATE KEY":
			return x509.ParsePKCS1PrivateKey(block.Bytes)
		case "EC PRIVATE KEY":
			return x509.ParseECPrivateKey(block.Bytes)
		}
	} else {
		switch block.Type {
		case "PUBLIC KEY":
			return x509.ParsePKIXPublicKey(block.Bytes)
		case "RSA PUBLIC KEY":
			return x509.ParsePKCS1PublicKey(block.Bytes)
		}
	}
	return nil, fmt.Errorf("PEM 类型 %q 与配置的密钥用途不符", block.Type)
}
