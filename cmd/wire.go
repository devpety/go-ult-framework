//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"ult/config"
	"ult/internal/api"
	"ult/internal/data"
	"ult/internal/data/repo"
	"ult/internal/infra/shared"
	"ult/internal/router"
	"ult/internal/server"
	"ult/internal/service"
	"ult/pkg/app"
	"ult/pkg/logger"

	"github.com/google/wire"
)

func initApp(ctx context.Context, conf *config.Config, appLogger *logger.Logger) (*app.App, func(), error) {
	panic(wire.Build(
		data.ProviderSet,
		repo.ProviderSet,
		service.ProviderSet,
		api.ProviderSet,
		router.ProviderSet,
		shared.ProviderSet,
		server.ProviderSet,
		newApp))
}
