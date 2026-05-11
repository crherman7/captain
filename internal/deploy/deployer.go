package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/apimachinery/pkg/runtime"
)

type Release struct {
	Name      string
	Namespace string
	Chart     string
	Values    map[string]interface{}
	// Hash, if non-empty, is stamped under values["captain"]["deployHash"] so
	// captain can skip subsequent deploys when nothing has changed. The
	// previous release's hash is read back from the in-cluster release values.
	Hash string
}

// DeployResult reports the outcome of a Deploy call.
type DeployResult struct {
	// Changed is false when the release already existed and its stored
	// captain.deployHash matched rel.Hash, meaning no upgrade was performed.
	Changed  bool
	Revision int
}

// StatusUpdate is a structured per-poll snapshot of a release in flight.
// It's emitted via HelmDeployer.OnStatus so the UI doesn't have to parse
// English log strings.
type StatusUpdate struct {
	// Action is what captain is doing right now: "installing", "upgrading",
	// "rolling back", or "uninstalling".
	Action string

	// Helm is the raw helm release status (deployed, pending-*, failed, ...).
	Helm release.Status

	// Description mirrors release.Info.Description — free-form English from
	// helm, useful as a fallback display string.
	Description string

	// ReadyPods / TotalPods are aggregated across Deployments and
	// StatefulSets in the release. TotalPods == 0 means no workloads were
	// found in the release manifest yet.
	ReadyPods int
	TotalPods int
}

type Deployer interface {
	Deploy(ctx context.Context, rel Release) (DeployResult, error)
	Uninstall(ctx context.Context, name, namespace string) error
}

type HelmDeployer struct {
	settings *cli.EnvSettings
	// OnStatus is invoked roughly every 500ms during an install/upgrade with a
	// snapshot polled from the helm SDK. Optional.
	OnStatus func(StatusUpdate)
}

func NewHelmDeployer(kubeContext string) *HelmDeployer {
	settings := cli.New()
	if kubeContext != "" {
		settings.KubeContext = kubeContext
	}
	return &HelmDeployer{settings: settings}
}

func (h *HelmDeployer) Deploy(ctx context.Context, rel Release) (DeployResult, error) {
	actionConfig := new(action.Configuration)

	h.settings.SetNamespace(rel.Namespace)

	if err := actionConfig.Init(
		h.settings.RESTClientGetter(),
		rel.Namespace,
		os.Getenv("HELM_DRIVER"),
		func(_ string, _ ...interface{}) {},
	); err != nil {
		return DeployResult{}, fmt.Errorf("initializing helm: %w", err)
	}

	chart, err := loader.Load(rel.Chart)
	if err != nil {
		return DeployResult{}, fmt.Errorf("loading chart %s: %w", rel.Chart, err)
	}

	stampHash(rel.Values, rel.Hash)

	// Pull enough history to find a recoverable prior revision for the
	// rollback path. Helm sorts oldest→newest.
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 10
	releases, err := histClient.Run(rel.Name)

	needsInstall := err == driver.ErrReleaseNotFound
	recovered := false

	if err == nil && len(releases) > 0 {
		latest := releases[len(releases)-1]
		status := latest.Info.Status

		switch {
		case status == release.StatusFailed:
			// Prefer rollback to a known-good revision; uninstall is
			// destructive (drops helm-managed PVCs in some charts and the
			// release history). Fall through to the normal upgrade path
			// after rollback so the caller's new values get applied.
			if rev := lastDeployedRevision(releases); rev > 0 {
				h.emitStatus(StatusUpdate{Action: "rolling back", Helm: status})
				rollback := action.NewRollback(actionConfig)
				rollback.Version = rev
				rollback.Wait = true
				rollback.Timeout = 2 * time.Minute
				if err := rollback.Run(rel.Name); err != nil {
					return DeployResult{}, fmt.Errorf("rolling back %s to rev %d: %w", rel.Name, rev, err)
				}
				recovered = true
			} else {
				h.emitStatus(StatusUpdate{Action: "recovering", Helm: status})
				uninstall := action.NewUninstall(actionConfig)
				_, _ = uninstall.Run(rel.Name)
				needsInstall = true
			}
		case status == release.StatusPendingInstall,
			status == release.StatusPendingUpgrade,
			status == release.StatusPendingRollback:
			// Stuck mid-operation. Rollback can't unstick this — only an
			// uninstall clears the pending lock.
			h.emitStatus(StatusUpdate{Action: "recovering", Helm: status})
			uninstall := action.NewUninstall(actionConfig)
			_, _ = uninstall.Run(rel.Name)
			needsInstall = true
		}
	} else if err != nil && err != driver.ErrReleaseNotFound {
		return DeployResult{}, fmt.Errorf("checking release %s: %w", rel.Name, err)
	}

	if needsInstall {
		install := action.NewInstall(actionConfig)
		install.ReleaseName = rel.Name
		install.Namespace = rel.Namespace
		install.CreateNamespace = true
		install.Wait = true
		install.Atomic = true
		install.Timeout = 5 * time.Minute

		installed, err := h.runWithPolling(ctx, actionConfig, rel.Name, "installing", func(c context.Context) (*release.Release, error) {
			return install.RunWithContext(c, chart, rel.Values)
		})
		if err != nil {
			return DeployResult{}, fmt.Errorf("installing %s: %w", rel.Name, err)
		}
		return DeployResult{Changed: true, Revision: installed.Version}, nil
	}

	// Skip the hash-equality short-circuit if we just rolled back — the
	// in-memory `releases` slice predates the rollback and may still show
	// the failed revision with a matching hash, which would incorrectly
	// mark the deploy as unchanged.
	if !recovered && rel.Hash != "" && len(releases) > 0 {
		if storedHash(releases[len(releases)-1].Config) == rel.Hash {
			return DeployResult{Changed: false, Revision: releases[len(releases)-1].Version}, nil
		}
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Wait = true
	upgrade.Atomic = true
	upgrade.Timeout = 5 * time.Minute
	upgrade.Namespace = rel.Namespace

	upgraded, err := h.runWithPolling(ctx, actionConfig, rel.Name, "upgrading", func(c context.Context) (*release.Release, error) {
		return upgrade.RunWithContext(c, rel.Name, chart, rel.Values)
	})
	if err != nil {
		return DeployResult{}, fmt.Errorf("upgrading %s: %w", rel.Name, err)
	}
	return DeployResult{Changed: true, Revision: upgraded.Version}, nil
}

// runWithPolling executes work in a goroutine and, while it runs, polls
// action.NewStatus every 500ms so the UI can show live release state.
func (h *HelmDeployer) runWithPolling(
	ctx context.Context,
	actionConfig *action.Configuration,
	name string,
	actionLabel string,
	work func(context.Context) (*release.Release, error),
) (*release.Release, error) {
	type result struct {
		rel *release.Release
		err error
	}

	h.emitStatus(StatusUpdate{Action: actionLabel, Helm: release.StatusPendingInstall})

	resultCh := make(chan result, 1)
	go func() {
		rel, err := work(ctx)
		resultCh <- result{rel, err}
	}()

	if h.OnStatus == nil {
		r := <-resultCh
		return r.rel, r.err
	}

	statusClient := action.NewStatus(actionConfig)
	statusClient.ShowResources = true

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case r := <-resultCh:
			return r.rel, r.err
		case <-ticker.C:
			rel, err := statusClient.Run(name)
			if err != nil || rel == nil || rel.Info == nil {
				continue
			}
			ready, total := countPods(rel.Info.Resources)
			h.OnStatus(StatusUpdate{
				Action:      actionLabel,
				Helm:        rel.Info.Status,
				Description: rel.Info.Description,
				ReadyPods:   ready,
				TotalPods:   total,
			})
		}
	}
}

func (h *HelmDeployer) emitStatus(u StatusUpdate) {
	if h.OnStatus != nil {
		h.OnStatus(u)
	}
}

// countPods aggregates ready/total pod counts across Deployments and
// StatefulSets in a release's resource set. Resources is map[kind][]object;
// we json-marshal each object to avoid a typed dependency on every k8s API
// group/version helm might return.
func countPods(resources map[string][]runtime.Object) (ready, total int) {
	for _, objs := range resources {
		for _, obj := range objs {
			data, err := json.Marshal(obj)
			if err != nil {
				continue
			}
			var probe struct {
				Kind string `json:"kind"`
				Spec struct {
					Replicas *int32 `json:"replicas"`
				} `json:"spec"`
				Status struct {
					Replicas      int32 `json:"replicas"`
					ReadyReplicas int32 `json:"readyReplicas"`
				} `json:"status"`
			}
			if err := json.Unmarshal(data, &probe); err != nil {
				continue
			}
			switch probe.Kind {
			case "Deployment", "StatefulSet", "ReplicaSet":
				want := int32(1)
				if probe.Spec.Replicas != nil {
					want = *probe.Spec.Replicas
				}
				total += int(want)
				ready += int(probe.Status.ReadyReplicas)
			}
		}
	}
	return ready, total
}

// lastDeployedRevision returns the version of the most recent release in
// history with status=deployed, or 0 if none exists. Helm history is sorted
// oldest→newest, so we walk backwards.
func lastDeployedRevision(releases []*release.Release) int {
	for i := len(releases) - 1; i >= 0; i-- {
		if releases[i].Info != nil && releases[i].Info.Status == release.StatusDeployed {
			return releases[i].Version
		}
	}
	return 0
}

func stampHash(values map[string]interface{}, hash string) {
	if hash == "" {
		return
	}
	cap, _ := values["captain"].(map[string]interface{})
	if cap == nil {
		cap = map[string]interface{}{}
		values["captain"] = cap
	}
	cap["deployHash"] = hash
}

func storedHash(values map[string]interface{}) string {
	cap, ok := values["captain"].(map[string]interface{})
	if !ok {
		return ""
	}
	h, _ := cap["deployHash"].(string)
	return h
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
