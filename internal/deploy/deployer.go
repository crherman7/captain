package deploy

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

type Release struct {
	Name      string
	Namespace string
	Chart     string
	Values    map[string]interface{}
}

type Deployer interface {
	Deploy(ctx context.Context, rel Release) error
	Uninstall(ctx context.Context, name, namespace string) error
}

type HelmDeployer struct {
	settings *cli.EnvSettings
	OnOutput func(string)
}

func NewHelmDeployer(kubeContext string) *HelmDeployer {
	settings := cli.New()
	if kubeContext != "" {
		settings.KubeContext = kubeContext
	}
	return &HelmDeployer{settings: settings}
}

func (h *HelmDeployer) Deploy(ctx context.Context, rel Release) error {
	actionConfig := new(action.Configuration)

	logger := func(_ string, _ ...interface{}) {}
	if h.OnOutput != nil {
		logger = func(format string, v ...interface{}) {
			msg := fmt.Sprintf(format, v...)
			msg = parseHelmLog(msg)
			if msg != "" {
				h.OnOutput(msg)
			}
		}
	}

	// Set namespace on settings so RESTClientGetter uses it for all K8s API calls
	h.settings.SetNamespace(rel.Namespace)

	if err := actionConfig.Init(
		h.settings.RESTClientGetter(),
		rel.Namespace,
		os.Getenv("HELM_DRIVER"),
		logger,
	); err != nil {
		return fmt.Errorf("initializing helm: %w", err)
	}

	chart, err := loader.Load(rel.Chart)
	if err != nil {
		return fmt.Errorf("loading chart %s: %w", rel.Chart, err)
	}

	// Check if release already exists to decide install vs upgrade
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	releases, err := histClient.Run(rel.Name)

	needsInstall := err == driver.ErrReleaseNotFound

	// If history exists but the release is in a stuck state, uninstall and reinstall
	if err == nil && len(releases) > 0 {
		status := releases[len(releases)-1].Info.Status
		if status == release.StatusPendingInstall ||
			status == release.StatusPendingUpgrade ||
			status == release.StatusPendingRollback ||
			status == release.StatusFailed {
			uninstall := action.NewUninstall(actionConfig)
			_, _ = uninstall.Run(rel.Name)
			needsInstall = true
		}
	} else if err != nil && err != driver.ErrReleaseNotFound {
		return fmt.Errorf("checking release %s: %w", rel.Name, err)
	}

	if needsInstall {
		install := action.NewInstall(actionConfig)
		install.ReleaseName = rel.Name
		install.Namespace = rel.Namespace
		install.CreateNamespace = true
		install.Wait = true
		install.Atomic = true
		install.Timeout = 5 * time.Minute

		if _, err := install.RunWithContext(ctx, chart, rel.Values); err != nil {
			return fmt.Errorf("installing %s: %w", rel.Name, err)
		}
		return nil
	}

	// Upgrade existing release
	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Wait = true
	upgrade.Atomic = true
	upgrade.Timeout = 5 * time.Minute
	upgrade.Namespace = rel.Namespace

	if _, err := upgrade.RunWithContext(ctx, rel.Name, chart, rel.Values); err != nil {
		return fmt.Errorf("upgrading %s: %w", rel.Name, err)
	}

	return nil
}

func (h *HelmDeployer) Uninstall(_ context.Context, name, namespace string) error {
	actionConfig := new(action.Configuration)

	h.settings.SetNamespace(namespace)

	if err := actionConfig.Init(
		h.settings.RESTClientGetter(),
		namespace,
		os.Getenv("HELM_DRIVER"),
		func(_ string, _ ...interface{}) {},
	); err != nil {
		return fmt.Errorf("initializing helm: %w", err)
	}

	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = true
	uninstall.Timeout = 2 * time.Minute

	if _, err := uninstall.Run(name); err != nil {
		return fmt.Errorf("uninstalling %s: %w", name, err)
	}
	return nil
}

// parseHelmLog extracts useful status from Helm SDK log lines.
func parseHelmLog(msg string) string {
	msg = strings.TrimSpace(msg)

	// Surface deployment readiness status
	if strings.Contains(msg, "not ready") {
		// "Deployment is not ready: default/api. 0 out of 1 expected pods are ready"
		if idx := strings.Index(msg, "."); idx > 0 {
			rest := strings.TrimSpace(msg[idx+1:])
			if rest != "" {
				return rest
			}
		}
		return msg
	}

	// Surface wait progress
	if strings.HasPrefix(msg, "beginning wait") {
		return "waiting for resources..."
	}
	if strings.Contains(msg, "wait for resources succeeded") {
		return "resources ready"
	}

	// Surface creating resources
	if strings.HasPrefix(msg, "creating") && strings.Contains(msg, "resource") {
		return msg
	}

	return ""
}
