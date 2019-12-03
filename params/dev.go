package params

import (
	"strings"

	"github.com/spf13/viper"
)

type devAccountInfo struct {
	Address    string
	KeyHex     string
	KeyContent string
}

// GetDevAccounts 获取开发测试账户私钥文件内容
// key 为地址
// Author: @ysqi
func GetDevAccounts() (list []devAccountInfo) {
	viper.UnmarshalKey("importKeys", &list) //忽略error
	//加工一次，使得JSON中编码正确
	for i, v := range list {
		list[i].KeyContent = strings.Replace(v.KeyContent, "'", "\"", -1)
	}
	return
}
