package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"ult/config"
	"ult/errcode"
	"ult/internal/infra/auth"
	"ult/internal/infra/shared"
	"ult/pkg/app"
	"ult/pkg/http"
	"ult/pkg/logger"

	utils_logger "github.com/devpety/go-utils/v3/logger"
)

func newApp(
	ctx context.Context,
	config *config.Config,
	appLogger *logger.Logger,
	hs *http.HTTPServer) *app.App {
	return app.NewApp(
		config,
		appLogger,
		app.WithAppContext(ctx),
		app.WithAppServer(hs))
}

func main() {
	configFile := flag.String("config", ".env.yml", "配置文件路径")
	flag.Parse()

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer stopSignals()
	
	// 首个信号触发取消后注销入口监听，保留再次发送信号强制退出的行为。
	stopRestore := context.AfterFunc(ctx, stopSignals)
	defer stopRestore()

	// 初始化配置
	conf, err := config.New(*configFile)
	if err != nil {
		panic(fmt.Sprintf("配置初始化失败：%v", err))
	}

	// 初始化 Environment
	conf.Environment = shared.NewEnvironment(conf).Value()

	// 打印启动信息
	printLogo(conf)

	// 初始化错误状态码
	errcode.NewRegistry(conf.Language.Local)

	// 初始化 Datetime
	var datetime = shared.NewDatetime(conf)

	// 初始化 Logger
	appLogger, err := logger.NewJSONLogger(
		// utils_logger.WithDisableConsole(),
		utils_logger.WithField(utils_logger.AppKey, conf.App.Name),
		utils_logger.WithField(utils_logger.EnvironmentKey, conf.Environment),
		utils_logger.WithTimeLayout(datetime.CSTLayout()),
		utils_logger.WithPathFileRotation(fmt.Sprintf("%s/runtime/logs/%s.log", conf.ProjectPath, conf.App.Name), utils_logger.PathFileRotationOption{
			MaxSize:    conf.Logger.MaxSize,
			MaxAge:     conf.Logger.MaxAge,
			MaxBackups: conf.Logger.MaxBackups,
			LocalTime:  conf.Logger.LocalTime,
			Compress:   conf.Logger.Compress,
		})) //	项目访问日志存放文件
	if err != nil {
		panic(fmt.Sprintf("日志初始化失败：%v", err))
	}

	// 启动期校验 JWT 密钥配置，避免配置错误延迟到首次鉴权才暴露。
	if _, err := auth.NewJWT(conf); err != nil {
		appLogger.UseApp(context.Background()).Error(fmt.Sprintf("JWT 初始化失败：%v", err))
		panic(err)
	}

	// 初始化应用服务
	application, cleanup, err := initApp(ctx, conf, appLogger)
	if err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			appLogger.UseApp(ctx).Info("依赖初始化已取消")
			return
		}
		appLogger.UseApp(context.Background()).Error(fmt.Sprintf("应用初始化失败：%v", err))
		panic(err)
	}

	defer func() {
		_ = appLogger.Sync()

		cleanup()
	}()

	// start and wait for stop signal
	if err := application.Run(); err != nil {
		appLogger.UseApp(context.Background()).Error(fmt.Sprintf("应用运行失败：%v", err))
		panic(err)
	}
}
