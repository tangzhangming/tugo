package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config tugo 项目配置
type Config struct {
	Project ProjectConfig `toml:"project"`
}

// ProjectConfig 项目配置
type ProjectConfig struct {
	Module string `toml:"module"` // 项目模块名，如 "com.company.demo"
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Project: ProjectConfig{
			Module: "App",
		},
	}
}

// FindAndLoad 从指定目录向上查找 tugo.toml 并加载
func FindAndLoad(startDir string) (*Config, string, error) {
	configPath := FindConfigFile(startDir)
	if configPath == "" {
		// 没找到配置文件，返回默认配置
		return DefaultConfig(), "", nil
	}

	config, err := Load(configPath)
	if err != nil {
		return nil, "", err
	}

	return config, configPath, nil
}

// FindConfigFile 从指定目录向上查找 tugo.toml
func FindConfigFile(startDir string) string {
	dir := startDir

	for {
		configPath := filepath.Join(dir, "tugo.toml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}

		// 获取父目录
		parent := filepath.Dir(dir)
		if parent == dir {
			// 已到根目录
			return ""
		}
		dir = parent
	}
}

// Load 加载配置文件
func Load(path string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, err
	}

	// 如果没有设置模块名，使用默认值
	if config.Project.Module == "" {
		config.Project.Module = "App"
	}

	return &config, nil
}

// GetProjectRoot 获取项目根目录（tugo.toml 所在目录）
func GetProjectRoot(configPath string) string {
	if configPath == "" {
		return ""
	}
	return filepath.Dir(configPath)
}

// ModuleToGoMod 将 tugo 模块名转换为 go mod 格式
// com.company.demo -> com.company.demo (保持不变，go mod 支持任意模块名)
func ModuleToGoMod(module string) string {
	return module
}

// PackagePathToGo 将 tugo 包路径转换为 Go import 路径
// com.company.demo.models -> com.company.demo/models
func PackagePathToGo(pkgPath string) string {
	// 找到最后一个点，分割为模块和子包
	// 这个函数用于将 tugo 的点分隔包路径转为 Go 的斜杠分隔
	// 简单实现：将第一个大写字母之前的最后一个点替换为斜杠
	// 更复杂的情况需要根据项目配置来判断

	// 暂时简单处理：将所有点替换为斜杠
	// 实际实现中需要结合项目配置的 module 来正确分割
	return pkgPath
}
