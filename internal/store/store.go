package store

import (
	"context"

	"github.com/logic-roastery/project-talos/internal/domain"
)

type AppStore interface {
	CreateApp(ctx context.Context, app *domain.App) error
	GetApp(ctx context.Context, id int64) (*domain.App, error)
	GetAppByName(ctx context.Context, name string) (*domain.App, error)
	GetAppByDomain(ctx context.Context, domain string) (*domain.App, error)
	GetAppByInstallationAndRepo(ctx context.Context, installationID, repoID int64) (*domain.App, error)
	ListApps(ctx context.Context) ([]*domain.App, error)
	UpdateApp(ctx context.Context, app *domain.App) error
	DeleteApp(ctx context.Context, id int64) error
	NextFallbackPort(ctx context.Context) (int, error)
}

type DeployStore interface {
	CreateDeploy(ctx context.Context, deploy *domain.Deploy) error
	GetDeploy(ctx context.Context, id int64) (*domain.Deploy, error)
	GetLatestDeploy(ctx context.Context, appID int64) (*domain.Deploy, error)
	GetLatestSuccessfulDeploy(ctx context.Context, appID int64) (*domain.Deploy, error)
	ListDeploys(ctx context.Context, appID int64, limit int) ([]*domain.Deploy, error)
	UpdateDeploy(ctx context.Context, deploy *domain.Deploy) error
}

type UserStore interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByID(ctx context.Context, id int64) (*domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	HasUsers(ctx context.Context) (bool, error)
}

type ServiceStore interface {
	CreateService(ctx context.Context, svc *domain.Service) error
	GetService(ctx context.Context, id int64) (*domain.Service, error)
	GetServiceByName(ctx context.Context, name string) (*domain.Service, error)
	ListServices(ctx context.Context) ([]*domain.Service, error)
	UpdateService(ctx context.Context, svc *domain.Service) error
	DeleteService(ctx context.Context, id int64) error

	LinkAppService(ctx context.Context, appID, serviceID int64, alias string) error
	UnlinkAppService(ctx context.Context, appID, serviceID int64) error
	ListAppServices(ctx context.Context, appID int64) ([]*domain.AppService, error)
	GetLinkedApps(ctx context.Context, serviceID int64) ([]*domain.AppService, error)

	SetAppEnvVar(ctx context.Context, envVar *domain.AppEnvVar) error
	GetAppEnvVars(ctx context.Context, appID int64) ([]*domain.AppEnvVar, error)
	DeleteAppEnvVar(ctx context.Context, appID int64, key string) error
}
