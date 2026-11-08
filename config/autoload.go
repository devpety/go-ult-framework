// Package config 提供配置管理功能。
// 从 YAML 配置文件加载应用配置，支持模块化配置结构。
package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"ult/config/autoload"

	"gopkg.in/yaml.v3"
)

// Config 应用配置结构体。
// 包含项目路径和所有模块配置。
type Config struct {
	*builder
	ProjectPath string // 配置文件所在目录，作为相对资源路径的基准
}

// builder 配置构建器结构体。
// 定义所有配置模块的字段映射。
type builder struct {
	Environment string                    `yaml:"environment"` // 运行环境 (dev/prod)
	App         autoload.App              `yaml:"app"`         // 应用配置
	Logger      autoload.Logger           `yaml:"logger"`      // 日志配置
	Language    autoload.Language         `yaml:"language"`    // 语言配置
	Validator   autoload.Validator        `yaml:"validator"`   // 验证器配置
	Server      autoload.Server           `yaml:"server"`      // 服务器配置
	DB          map[string]autoload.DB    `yaml:"db"`          // 数据库配置（支持多连接）
	Redis       map[string]autoload.Redis `yaml:"redis"`       // Redis 配置（支持多连接）
	JWT         autoload.JWT              `yaml:"jwt"`         // JWT 认证配置
	Datetime    autoload.Datetime         `yaml:"datetime"`    // 日期时间配置
	Notify      autoload.Notify           `yaml:"notify"`      // 告警通知配置
}

// New 从显式路径加载配置；相对路径按进程工作目录解析。
func New(filename string) (*Config, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, fmt.Errorf("配置文件路径不能为空")
	}
	filename, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("解析配置路径失败: %w", err)
	}
	conf := &Config{ProjectPath: filepath.Dir(filename)}
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件 %s 失败: %w", filename, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(&conf.builder); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", filename, err)
	}
	if conf.builder == nil {
		return nil, fmt.Errorf("配置文件 %s 不能为空", filename)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", filename, err)
		}
		return nil, fmt.Errorf("配置文件 %s 只能包含一个 YAML 文档", filename)
	}
	return conf, nil
}
