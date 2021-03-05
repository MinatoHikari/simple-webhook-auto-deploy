package main

import (
	"errors"
	"fmt"
	"github.com/spf13/viper"
)

// Config 配置.
var Config *viper.Viper

// InitConfig 读取配置文件，返回是否找得到配置文件
func InitConfig() (notFound bool) {
	Config = viper.New()
	Config.SetConfigFile("./config.toml")
	Config.SetConfigName("config")
	Config.SetConfigType("toml")
	Config.AddConfigPath(".")

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
func GetEnv() (path string, script string, dist string, branch string) {
	path = Config.GetString("output")
	script = Config.GetString("script")
	dist = Config.GetString("dist")
	branch = Config.GetString("branch")

	return
}

// GetToken 获取token.
func GetToken() *string {
	token := Config.GetString("token")

	return &token
}
