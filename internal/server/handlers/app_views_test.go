package handlers

import (
	"testing"
	"time"

	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestBuildDashboardAppView(t *testing.T) {
	tests := []struct {
		name         string
		app          *domain.App
		proxyMode    config.ProxyMode
		summaryLabel string
		summaryValue string
		routeValue   string
		routeHint    string
	}{
		{
			name: "managed app uses branch and fallback port",
			app: &domain.App{
				ID:           1,
				Name:         "managed",
				AppType:      domain.AppTypeManaged,
				Branch:       "main",
				FallbackPort: 40001,
				AccessMode:   domain.AccessModePort,
				AccessURL:    "http://talos.local:40001",
				UpdatedAt:    time.Now(),
			},
			proxyMode:    config.ProxyModeInternal,
			summaryLabel: "Branch",
			summaryValue: "main",
			routeValue:   "Port 40001",
			routeHint:    "Fallback port mode",
		},
		{
			name: "adopted app uses container and manual route hint",
			app: &domain.App{
				ID:            2,
				Name:          "adopted",
				AppType:       domain.AppTypeAdoptedContainer,
				ContainerName: "nginx",
				Domain:        "adopted.example.com",
				UpdatedAt:     time.Now(),
			},
			proxyMode:    config.ProxyModeExternal,
			summaryLabel: "Container",
			summaryValue: "nginx",
			routeValue:   "adopted.example.com",
			routeHint:    "Manual external route pending",
		},
		{
			name: "external app uses target summary",
			app: &domain.App{
				ID:             3,
				Name:           "external",
				AppType:        domain.AppTypeExternalService,
				ExternalTarget: "http://10.0.0.10:8080",
				UpdatedAt:      time.Now(),
			},
			proxyMode:    config.ProxyModeInternal,
			summaryLabel: "Target",
			summaryValue: "http://10.0.0.10:8080",
			routeValue:   "http://10.0.0.10:8080",
			routeHint:    "Direct external target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDashboardAppView(tt.app, tt.proxyMode)
			if got.SummaryLabel != tt.summaryLabel {
				t.Fatalf("summary label = %q, want %q", got.SummaryLabel, tt.summaryLabel)
			}
			if got.SummaryValue != tt.summaryValue {
				t.Fatalf("summary value = %q, want %q", got.SummaryValue, tt.summaryValue)
			}
			if got.RouteValue != tt.routeValue {
				t.Fatalf("route value = %q, want %q", got.RouteValue, tt.routeValue)
			}
			if got.RouteHint != tt.routeHint {
				t.Fatalf("route hint = %q, want %q", got.RouteHint, tt.routeHint)
			}
		})
	}
}

func TestBuildAppDetailPageDataCapabilities(t *testing.T) {
	managed := buildAppDetailPageData(nil, &domain.App{
		Name:    "managed",
		AppType: domain.AppTypeManaged,
		RepoURL: "https://github.com/acme/managed",
		Branch:  "main",
	}, nil, true, nil, config.ProxyModeInternal, "")
	if !managed.ShowDeployControls || !managed.ShowDeployHistory || !managed.ShowLogs {
		t.Fatalf("managed app should expose deploy controls, deploy history, and logs")
	}
	if managed.ShowRestart {
		t.Fatalf("managed app should not expose restart action")
	}

	adopted := buildAppDetailPageData(nil, &domain.App{
		Name:          "adopted",
		AppType:       domain.AppTypeAdoptedContainer,
		ContainerName: "web",
		Domain:        "adopted.example.com",
	}, nil, false, nil, config.ProxyModeExternal, "snippet")
	if adopted.ShowDeployControls || adopted.ShowDeployHistory {
		t.Fatalf("adopted app should hide managed deploy surfaces")
	}
	if !adopted.ShowRestart || !adopted.ShowLogs {
		t.Fatalf("adopted app should expose restart and logs")
	}
	if adopted.ManualRouteSnippet == "" {
		t.Fatalf("adopted app should keep manual route snippet when provided")
	}

	external := buildAppDetailPageData(nil, &domain.App{
		Name:           "external",
		AppType:        domain.AppTypeExternalService,
		ExternalTarget: "http://10.0.0.12:9000",
	}, nil, false, nil, config.ProxyModeInternal, "")
	if external.ShowLogs || external.ShowRestart || external.ShowRuntime {
		t.Fatalf("external service should hide logs, restart, and runtime sections")
	}
}

func TestBuildAppSettingsPageDataByType(t *testing.T) {
	managed := buildAppSettingsPageData(nil, &domain.App{
		Name:    "managed",
		AppType: domain.AppTypeManaged,
		RepoURL: "https://github.com/acme/managed",
		Branch:  "main",
	}, nil, nil, nil, config.ProxyModeInternal, "")
	if !managed.ShowEnvVars || !managed.ShowLinkedServices {
		t.Fatalf("managed settings should keep env vars and linked services")
	}

	external := buildAppSettingsPageData(nil, &domain.App{
		Name:           "external",
		AppType:        domain.AppTypeExternalService,
		ExternalTarget: "http://10.0.0.12:9000",
	}, nil, nil, nil, config.ProxyModeExternal, "snippet")
	if external.ShowEnvVars || external.ShowLinkedServices {
		t.Fatalf("external settings should hide managed-only surfaces")
	}
	if !external.ShowManualRoute {
		t.Fatalf("external settings should expose manual route snippet when provided")
	}
}
