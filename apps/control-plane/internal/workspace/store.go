package workspace

import (
	"context"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

type Store interface {
	CreateWorkspace(context.Context, contracts.Workspace) (contracts.Workspace, error)
	GetWorkspace(context.Context, string) (contracts.Workspace, error)
	HasCurrentInstallation(context.Context, string, string) (bool, error)
	CreateInstallation(context.Context, string, contracts.Installation) (contracts.Installation, error)
	GetInstallation(context.Context, string, string) (contracts.Installation, error)
	GetCurrentInstallation(context.Context, string, string) (contracts.Installation, error)
	ListInstallations(context.Context, string, int, *InstallationPosition) ([]contracts.Installation, bool, error)
	ChangeInstallationStatus(context.Context, string, string, string, time.Time) (contracts.Installation, error)
	UninstallInstallation(context.Context, string, string, time.Time) (contracts.Installation, error)
	Check(context.Context) error
}
