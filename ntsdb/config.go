// Author: @ysqi

package ntsdb

// Config 数据库配置信息
type Config struct {
	Storage  string // 存储格式：SqlLite或者MySql
	Name     string // 数据库名,不允许为空
	Server   string // 主机地址,为空时默认为：localhost:3306
	UserName string // 用户名
	Password string // 密码
	Args     string // 更多参数，MYSQL数据库连接时支持大量参数设置，格式：item1=value1&item2=value2
}

const (
	SQLite = "sqlite"
	MySQL  = "mysql"
)
