package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
)

type appTypeOption struct {
	Value           domain.AppType
	Label           string
	OwnershipLabel  string
	Summary         string
	OperationalCopy string
	SupportsDeploys bool
	SupportsLogs    bool
	SupportsRestart bool
	SupportsDomains bool
}

type appCapabilityView struct {
	Label   string
	State   string
	Enabled bool
}

type appInfoItemView struct {
	Label string
	Value string
	Hint  string
	Link  string
}

type dashboardAppView struct {
	ID             int64
	Name           string
	Status         domain.AppStatus
	TypeLabel      string
	OwnershipLabel string
	SummaryLabel   string
	SummaryValue   string
	RouteValue     string
	RouteHint      string
	RouteLink      string
	UpdatedAt      time.Time
}

type appCreatePageData struct {
	GitHubConfigured bool
	Repos            []RepoInfo
	RepoError        string
	ProxyMode        config.ProxyMode
	Containers       []docker.ContainerInfo
	TypeOptions      []appTypeOption
}

type appDetailPageData struct {
	User                any
	App                 *domain.App
	Deploys             []*domain.Deploy
	GitHubConfigured    bool
	RuntimeInfo         *docker.ContainerInfo
	ManualRouteSnippet  string
	TypeLabel           string
	OwnershipLabel      string
	OwnershipCopy       string
	IdentitySummary     string
	Capabilities        []appCapabilityView
	HeaderItems         []appInfoItemView
	RuntimeItems        []appInfoItemView
	RoutingItems        []appInfoItemView
	TechnicalItems      []appInfoItemView
	ShowSettings        bool
	ShowRestart         bool
	ShowLogs            bool
	ShowGitHub          bool
	ShowDeployControls  bool
	ShowDeployHistory   bool
	ShowRuntime         bool
	ShowTargetMetadata  bool
	ShowOwnershipNotice bool
	TargetTitle         string
	TargetItems         []appInfoItemView
}

type appSettingsPageData struct {
	User               any
	App                *domain.App
	EnvVars            []*domain.AppEnvVar
	Links              []*domain.AppService
	AllServices        []*domain.Service
	TypeLabel          string
	OwnershipLabel     string
	Intro              string
	RoutingItems       []appInfoItemView
	OwnershipItems     []appInfoItemView
	ShowLinkedServices bool
	ShowEnvVars        bool
	ShowManualRoute    bool
	ManualRouteSnippet string
}

func appTypeOptions() []appTypeOption {
	return []appTypeOption{
		appTypeMeta(domain.AppTypeManaged),
		appTypeMeta(domain.AppTypeAdoptedContainer),
		appTypeMeta(domain.AppTypeExternalService),
	}
}

func appTypeMeta(appType domain.AppType) appTypeOption {
	switch appType {
	case domain.AppTypeAdoptedContainer:
		return appTypeOption{
			Value:           appType,
			Label:           "Adopted Container",
			OwnershipLabel:  "Observed by Talos",
			Summary:         "Talos watches an existing Docker container and can route, restart, and read logs from it.",
			OperationalCopy: "Talos does not build or deploy this container.",
			SupportsLogs:    true,
			SupportsRestart: true,
			SupportsDomains: true,
		}
	case domain.AppTypeExternalService:
		return appTypeOption{
			Value:           appType,
			Label:           "External Service",
			OwnershipLabel:  "Externally owned",
			Summary:         "Talos registers a URL or host:port target and can present routing details for it.",
			OperationalCopy: "Talos does not run, restart, or stream logs from this service.",
			SupportsDomains: true,
		}
	default:
		return appTypeOption{
			Value:           domain.AppTypeManaged,
			Label:           "Managed App",
			OwnershipLabel:  "Talos-managed",
			Summary:         "Talos deploys this app from a repository or image reference and manages releases end to end.",
			OperationalCopy: "Deploys, logs, rollback, domains, and GitHub automation are available.",
			SupportsDeploys: true,
			SupportsLogs:    true,
			SupportsDomains: true,
		}
	}
}

func buildDashboardAppView(app *domain.App, proxyMode config.ProxyMode) dashboardAppView {
	summaryLabel := "Branch"
	summaryValue := valueOrDash(app.Branch)
	switch app.AppType {
	case domain.AppTypeAdoptedContainer:
		summaryLabel = "Container"
		summaryValue = valueOrDash(app.EffectiveContainerName())
	case domain.AppTypeExternalService:
		summaryLabel = "Target"
		summaryValue = valueOrDash(app.ExternalTarget)
	}

	routeValue := "Not published"
	routeHint := "Fallback port unavailable"
	routeLink := ""
	switch {
	case app.Domain != "":
		routeValue = app.Domain
		routeLink = "https://" + app.Domain
		if proxyMode == config.ProxyModeExternal && app.AppType != domain.AppTypeManaged {
			routeHint = "Manual external route pending"
		} else {
			routeHint = "Published domain"
		}
	case app.AccessMode == domain.AccessModePort && app.FallbackPort > 0:
		routeValue = fmt.Sprintf(":%d", app.FallbackPort)
		routeHint = "Fallback port mode"
		routeLink = app.AccessURL
	case app.AppType == domain.AppTypeExternalService && app.ExternalTarget != "":
		routeValue = app.ExternalTarget
		routeHint = "Direct external target"
		routeLink = app.ExternalTarget
	case app.AccessURL != "":
		routeValue = app.AccessURL
		routeHint = string(app.AccessMode)
		routeLink = app.AccessURL
	}

	meta := appTypeMeta(app.AppType)
	return dashboardAppView{
		ID:             app.ID,
		Name:           app.Name,
		Status:         app.Status,
		TypeLabel:      meta.Label,
		OwnershipLabel: meta.OwnershipLabel,
		SummaryLabel:   summaryLabel,
		SummaryValue:   summaryValue,
		RouteValue:     routeValue,
		RouteHint:      routeHint,
		RouteLink:      routeLink,
		UpdatedAt:      app.UpdatedAt,
	}
}

func buildAppDetailPageData(user any, app *domain.App, deploys []*domain.Deploy, githubConfigured bool, runtimeInfo *docker.ContainerInfo, proxyMode config.ProxyMode, manualRouteSnippet string) appDetailPageData {
	meta := appTypeMeta(app.AppType)
	capabilities := []appCapabilityView{
		{Label: "Deploys", Enabled: meta.SupportsDeploys},
		{Label: "Logs", Enabled: meta.SupportsLogs},
		{Label: "Restart", Enabled: meta.SupportsRestart},
		{Label: "Domains", Enabled: meta.SupportsDomains},
	}
	for i := range capabilities {
		if capabilities[i].Enabled {
			capabilities[i].State = "Available"
		} else {
			capabilities[i].State = "Hidden"
		}
	}

	headerItems := []appInfoItemView{
		{Label: "Route", Value: detailRouteValue(app), Hint: detailRouteHint(app, proxyMode), Link: detailRouteLink(app)},
		{Label: "Ownership", Value: meta.OwnershipLabel},
		{Label: "Edge", Value: string(app.EdgeProvider)},
		{Label: "Status", Value: string(app.Status)},
	}

	routingItems := []appInfoItemView{
		{Label: "Access mode", Value: string(app.AccessMode)},
		{Label: "Published domain", Value: valueOrDash(app.Domain)},
		{Label: "Public URL", Value: valueOrDash(app.AccessURL), Link: safeLink(app.AccessURL)},
	}
	if app.FallbackPort > 0 {
		routingItems = append(routingItems, appInfoItemView{Label: "Fallback port", Value: fmt.Sprintf("%d", app.FallbackPort)})
	}

	runtimeItems := []appInfoItemView{
		{Label: "Container", Value: firstNonEmpty(runtimeContainerName(runtimeInfo), app.EffectiveContainerName(), "—")},
		{Label: "Internal port", Value: intValueOrDash(app.InternalPort)},
		{Label: "Image", Value: firstNonEmpty(runtimeImage(runtimeInfo), app.ImageRef, "—")},
		{Label: "Networks", Value: firstNonEmpty(runtimeNetworks(runtimeInfo), app.DockerNetwork, "—")},
	}

	technicalItems := []appInfoItemView{
		{Label: "Source", Value: valueOrDash(app.Source)},
		{Label: "Runtime owner", Value: string(app.RuntimeOwner)},
		{Label: "Live container", Value: valueOrDash(app.LiveContainerName)},
		{Label: "Updated", Value: app.UpdatedAt.Format(time.RFC3339)},
	}

	identitySummary := appIdentitySummary(app)
	targetTitle := "Target"
	targetItems := []appInfoItemView{}
	switch app.AppType {
	case domain.AppTypeManaged:
		targetTitle = "Release Source"
		targetItems = []appInfoItemView{
			{Label: "Repository", Value: valueOrDash(app.RepoURL), Link: safeLink(app.RepoURL)},
			{Label: "Branch", Value: valueOrDash(app.Branch)},
			{Label: "Image ref", Value: valueOrDash(app.ImageRef)},
		}
	case domain.AppTypeAdoptedContainer:
		targetTitle = "Adopted Runtime"
		targetItems = []appInfoItemView{
			{Label: "Container name", Value: valueOrDash(app.EffectiveContainerName())},
			{Label: "Docker network", Value: valueOrDash(app.DockerNetwork)},
			{Label: "Talos role", Value: "Observed runtime with optional routing"},
		}
	case domain.AppTypeExternalService:
		targetItems = []appInfoItemView{
			{Label: "External target", Value: valueOrDash(app.ExternalTarget), Link: safeLink(app.ExternalTarget)},
			{Label: "Talos role", Value: "Routes and tracks access only"},
		}
	}

	return appDetailPageData{
		User:                user,
		App:                 app,
		Deploys:             deploys,
		GitHubConfigured:    githubConfigured,
		RuntimeInfo:         runtimeInfo,
		ManualRouteSnippet:  manualRouteSnippet,
		TypeLabel:           meta.Label,
		OwnershipLabel:      meta.OwnershipLabel,
		OwnershipCopy:       meta.OperationalCopy,
		IdentitySummary:     identitySummary,
		Capabilities:        capabilities,
		HeaderItems:         headerItems,
		RuntimeItems:        runtimeItems,
		RoutingItems:        routingItems,
		TechnicalItems:      technicalItems,
		ShowSettings:        true,
		ShowRestart:         meta.SupportsRestart,
		ShowLogs:            meta.SupportsLogs,
		ShowGitHub:          app.AppType == domain.AppTypeManaged,
		ShowDeployControls:  app.AppType == domain.AppTypeManaged,
		ShowDeployHistory:   app.AppType == domain.AppTypeManaged,
		ShowRuntime:         app.AppType != domain.AppTypeExternalService,
		ShowTargetMetadata:  true,
		ShowOwnershipNotice: app.AppType != domain.AppTypeManaged,
		TargetTitle:         targetTitle,
		TargetItems:         targetItems,
	}
}

func buildAppSettingsPageData(user any, app *domain.App, envVars []*domain.AppEnvVar, links []*domain.AppService, allServices []*domain.Service, proxyMode config.ProxyMode, manualRouteSnippet string) appSettingsPageData {
	meta := appTypeMeta(app.AppType)
	intro := "Routing, runtime, and linked configuration for this app."
	if app.AppType == domain.AppTypeManaged {
		intro = "Environment variables and linked services for Talos-managed deploys."
	}

	ownershipItems := []appInfoItemView{
		{Label: "App type", Value: meta.Label},
		{Label: "Ownership", Value: meta.OwnershipLabel},
	}
	switch app.AppType {
	case domain.AppTypeManaged:
		ownershipItems = append(ownershipItems,
			appInfoItemView{Label: "Repository", Value: valueOrDash(app.RepoURL), Link: safeLink(app.RepoURL)},
			appInfoItemView{Label: "Branch", Value: valueOrDash(app.Branch)},
		)
	case domain.AppTypeAdoptedContainer:
		ownershipItems = append(ownershipItems,
			appInfoItemView{Label: "Container", Value: valueOrDash(app.EffectiveContainerName())},
			appInfoItemView{Label: "Docker network", Value: valueOrDash(app.DockerNetwork)},
		)
	case domain.AppTypeExternalService:
		ownershipItems = append(ownershipItems,
			appInfoItemView{Label: "External target", Value: valueOrDash(app.ExternalTarget), Link: safeLink(app.ExternalTarget)},
			appInfoItemView{Label: "Runtime owner", Value: "External"},
		)
	}

	routingItems := []appInfoItemView{
		{Label: "Route", Value: detailRouteValue(app), Hint: detailRouteHint(app, proxyMode), Link: detailRouteLink(app)},
		{Label: "Access mode", Value: string(app.AccessMode)},
		{Label: "Public URL", Value: valueOrDash(app.AccessURL), Link: safeLink(app.AccessURL)},
	}
	if app.FallbackPort > 0 {
		routingItems = append(routingItems, appInfoItemView{Label: "Fallback port", Value: fmt.Sprintf("%d", app.FallbackPort)})
	}

	return appSettingsPageData{
		User:               user,
		App:                app,
		EnvVars:            envVars,
		Links:              links,
		AllServices:        allServices,
		TypeLabel:          meta.Label,
		OwnershipLabel:     meta.OwnershipLabel,
		Intro:              intro,
		RoutingItems:       routingItems,
		OwnershipItems:     ownershipItems,
		ShowLinkedServices: app.AppType != domain.AppTypeExternalService,
		ShowEnvVars:        app.AppType == domain.AppTypeManaged,
		ShowManualRoute:    manualRouteSnippet != "",
		ManualRouteSnippet: manualRouteSnippet,
	}
}

func appIdentitySummary(app *domain.App) string {
	switch app.AppType {
	case domain.AppTypeAdoptedContainer:
		return fmt.Sprintf("Talos observes the %s container and exposes only the controls that apply to an externally owned runtime.", valueOrDash(app.EffectiveContainerName()))
	case domain.AppTypeExternalService:
		return fmt.Sprintf("Talos routes traffic toward %s but does not own the runtime behind it.", valueOrDash(app.ExternalTarget))
	default:
		return fmt.Sprintf("Talos manages releases for %s from %s.", app.Name, valueOrDash(app.RepoURL))
	}
}

func detailRouteValue(app *domain.App) string {
	switch {
	case app.Domain != "":
		return app.Domain
	case app.FallbackPort > 0:
		return fmt.Sprintf(":%d", app.FallbackPort)
	case app.AccessURL != "":
		return app.AccessURL
	default:
		return "Not published"
	}
}

func detailRouteHint(app *domain.App, proxyMode config.ProxyMode) string {
	switch {
	case app.Domain != "" && proxyMode == config.ProxyModeExternal && app.AppType != domain.AppTypeManaged:
		return "Manual external Traefik config required"
	case app.Domain != "":
		return "Published domain"
	case app.FallbackPort > 0:
		return "Fallback port access"
	case app.AppType == domain.AppTypeExternalService:
		return "Direct target only"
	default:
		return "No published route"
	}
}

func detailRouteLink(app *domain.App) string {
	if app.Domain != "" {
		return "https://" + app.Domain
	}
	return safeLink(app.AccessURL)
}

func runtimeContainerName(info *docker.ContainerInfo) string {
	if info == nil {
		return ""
	}
	return info.Name
}

func runtimeImage(info *docker.ContainerInfo) string {
	if info == nil {
		return ""
	}
	return info.Image
}

func runtimeNetworks(info *docker.ContainerInfo) string {
	if info == nil {
		return ""
	}
	return strings.Join(info.Networks, ", ")
}

func valueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "—"
	}
	return v
}

func intValueOrDash(v int) string {
	if v <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d", v)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func safeLink(v string) string {
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	return ""
}
