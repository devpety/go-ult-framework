package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"ult/config"
	genDb "ult/generate/gormgen/db"
	"ult/pkg/db"
	"ult/pkg/logger"
	"ult/pkg/repositories"
	"ult/pkg/startup"

	utils_logger "github.com/devpety/go-utils/v3/logger"
	"github.com/devpety/go-utils/v3/server/system"
)

func main() {
	configFile := flag.String("config", ".env.yml", "配置文件路径")
	outPath := flag.String("out", "internal/data/dbquery", "查询器输出目录")
	flag.Parse()

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	
	stopRestore := context.AfterFunc(ctx, stopSignals)
	defer stopRestore()

	// 初始化配置
	conf, err := config.New(*configFile)
	if err != nil {
		panic(err)
	}

	// 初始化 Environment
	var environment = system.NewEnvironment(conf.Environment)
	conf.Environment = environment.Value()

	// 初始化 Datetime
	var datetime = system.NewDatetime(
		system.WithLocation(conf.Datetime.Location),
		system.WithCSTLayout(conf.Datetime.CSTLayout))

	log, err := logger.NewJSONLogger(
		utils_logger.WithField(utils_logger.AppKey, conf.App.Name),
		utils_logger.WithField(utils_logger.EnvironmentKey, conf.Environment),
		utils_logger.WithTimeLayout(datetime.CSTLayout()))
	if err != nil {
		panic(err)
	}

	timeout, err := startup.Timeout(conf.App.StartupTimeoutMS, startup.DefaultTimeout)
	if err != nil {
		panic(err)
	}
	initCtx, cancel := context.WithTimeout(ctx, timeout)
	defaultDB, err := db.NewDb(initCtx, repositories.DbConnectionDefaultName, conf.DB[repositories.DbConnectionDefaultName], log)
	cancel()
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := defaultDB.Close(); err != nil {
			log.UseApp(ctx).Error(err.Error())
		}
	}()

	// 执行生成默认数据库对应的模型文件
	genDb.NewGeneratorDefaultDb(defaultDB, *outPath)
}
