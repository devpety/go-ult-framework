//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"ult/config"
	"ult/internal/api"
	"ult/internal/app"
	"ult/internal/data"
	"ult/internal/data/repo"
	"ult/internal/router"
	"ult/internal/server"
	"ult/internal/service"
	pkgapp "ult/pkg/app"

	"github.com/google/wire"
)

func initApp(ctx context.Context, conf *config.Config, tools *app.Tools) (*pkgapp.App, func(), error) {
	panic(wire.Build(
		data.ProviderSet,
		repo.ProviderSet,
		service.ProviderSet,
		api.ProviderSet,
		router.ProviderSet,
		server.ProviderSet,
		newApp))
}
