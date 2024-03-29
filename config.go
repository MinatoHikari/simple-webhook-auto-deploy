package main

import (
	"errors"
	"fmt"
	"github.com/spf13/viper"
	"strings"
)

type ConfigEnv struct {
	Path                 string
	Script               string
	Dist                 string
	Branch               string
	LogPath              string
	PackageManager       string
	AdditionalScript     string
	AdditionalScriptArgs []string
}

// Config 配置.
var Config *viper.Viper

// InitConfig 读取配置文件，返回是否找得到配置文件
func InitConfig() (notFound bool) {
	Config = viper.New()
	Config.SetConfigFile("./config.toml")
	//Config.SetConfigName("config")
	//Config.SetConfigType("toml")
	//Config.AddConfigPath(".")

	err := Config.ReadInConfig() // 查找并读取配置文件
	if err == nil {
		return
	}

	notFound = false

	var notFoundError viper.ConfigFileNotFoundError

	ok := errors.Is(err, notFoundError)
	if ok {
		// 配置文件未找到错误；如果需要可以忽略
		notFound = true

		return
	}
	// 配置文件被找到，但产生了另外的错误
	panic(fmt.Errorf("Fatal error config file: %s \n", err))
}

// GetEnv 获取输出目录 要运行的npm命令 项目的dist编译目录.
func GetEnv() *ConfigEnv {
	configEnv := new(ConfigEnv)
	configEnv.Path = Config.GetString("output")
	configEnv.Script = Config.GetString("script")
	configEnv.Dist = Config.GetString("dist")
	configEnv.Branch = Config.GetString("branch")
	configEnv.LogPath = Config.GetString("logPath")
	configEnv.PackageManager = Config.GetString("package-manager")
	configEnv.AdditionalScript = Config.GetString("additional-script")
	configEnv.AdditionalScriptArgs = Config.GetStringSlice("additional-script-args")

	return configEnv
}

// GetToken 获取token.
func GetToken() *string {
	token := Config.GetString("token")

	return &token
}

// GetPort 获取服务运行的端口
func GetPort() string {
	port := Config.GetString("port")
	if port != "" {
		port = strings.Join([]string{":", port}, "")
	} else {
		port = ":8079"
	}

	return port
}
