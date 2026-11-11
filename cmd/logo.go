package main

import (
	"fmt"
	"runtime"
	"ult/config"

	"github.com/fatih/color"
)

// printLogo 打印项目服务启动信息。
// 显示项目 Logo、Go 版本、系统信息、服务配置等信息。
//
// 参数:
//   - conf: 应用配置
func printLogo(conf *config.Config) {
	// see https://patorjk.com/software/taag/#p=testall&f=Graffiti&t=ult
	var logo = `
	██╗   ██╗	██╗  	     ████████╗
	██║   ██║	██║  	     ╚══██╔══╝
	██║   ██║	██║     	██║   
	██║   ██║	██║     	██║   
	╚██████╔╝	███████╗	██║   
	 ╚═════╝ 	╚══════╝	╚═╝
`
	color.HiYellow(logo)

	contents := fmt.Sprintf(`
GO 版本及路径: %s (%s)
系统类型及架构: %s (%s) - %d 核 CPU
服务名称: %s (%s)
服务版本: %s
服务环境: %s
	`,
		runtime.Version(),
		runtime.GOROOT(),
		runtime.GOOS,
		runtime.GOARCH,
		runtime.GOMAXPROCS(0),

		conf.App.Name,
		conf.App.ID,
		conf.App.Version,
		conf.Environment,
	)

	color.HiGreen(contents)
}
