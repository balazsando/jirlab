package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/integration"
)

// --- Message types ---

type kubeConfigsLoadedMsg struct {
	configs []kubeConfig
}

type podsLoadedMsg struct {
	pods []kubePod
	err  error
}

type servicesLoadedMsg struct {
	services []kubeService
	err      error
}

type deploymentsLoadedMsg struct {
	deployments []kubeDeployment
	err         error
}

type kubeRefreshMsg struct{}

type kubeActionDoneMsg struct{ message string }

// --- Pane enum ---

type kubePane int

const (
	kubePaneConfigs kubePane = iota
	kubePanePods
	kubePaneServices
	kubePaneDeployments
)

// kubePaneLabels are the bottom resource tab labels (Configs pane is the top table, not a bottom tab).
var kubePaneLabels = []string{"Pods", "Services", "Deployments"}

// --- Data models ---

type kubeConfig struct {
	Name         string
	Path         string
	IsMonitoring bool
}

type kubePod struct {
	Name    string
	Ready   string
	Version string
	Status  string
	Age     string
}

type kubeService struct {
	Name       string
	Type       string
	ClusterIP  string
	ExternalIP string
	Ports      string
	Age        string
}

type kubeDeployment struct {
	Name      string
	Ready     string
	UpToDate  string
	Available string
	Age       string
}

// --- KubeSection ---

type KubeSection struct {
	configs     []kubeConfig
	pods        []kubePod
	services    []kubeService
	deployments []kubeDeployment

	activeConfig     string
	activePane       kubePane // tracks keyboard focus: Configs or resource pane
	lastResourcePane kubePane // which resource tab is displayed in the bottom section

	kubectl integration.KubectlService

	cursor    int // config list cursor
	podCursor int
	svcCursor int
	depCursor int

	loading    bool
	svcLoading bool
	depLoading bool

	podErr string
	svcErr string
	depErr string

	statusMsg  string
	termHeight int // terminal height, set on resize
}

func newKubeSection(kubectl integration.KubectlService) KubeSection {
	return KubeSection{
		activeConfig:     os.Getenv("KUBECONFIG"),
		loading:          true,
		lastResourcePane: kubePanePods,
		kubectl:          kubectl,
	}
}

func kubeInitCmd() tea.Cmd {
	return func() tea.Msg {
		return kubeConfigsLoadedMsg{configs: discoverKubeConfigs()}
	}
}

// discoverKubeConfigs globs ~/.kube/config-*.yaml and config-*.yml.
// Priority order: test → preprod → prod, then rest alphabetically.
func discoverKubeConfigs() []kubeConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	kubeDir := filepath.Join(home, ".kube")

	patterns := []string{
		filepath.Join(kubeDir, "config-*.yaml"),
		filepath.Join(kubeDir, "config-*.yml"),
	}
	seen := make(map[string]bool)
	var configs []kubeConfig
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, p := range matches {
			if seen[p] {
				continue
			}
			seen[p] = true
			name := filepath.Base(p)
			name = strings.TrimSuffix(name, ".yaml")
			name = strings.TrimSuffix(name, ".yml")
			name = strings.TrimPrefix(name, "config-")
			configs = append(configs, kubeConfig{
				Name:         name,
				Path:         p,
				IsMonitoring: strings.Contains(name, "monitoring"),
			})
		}
	}

	sort.SliceStable(configs, func(i, j int) bool {
		pi := kubeConfigPriority(configs[i])
		pj := kubeConfigPriority(configs[j])
		if pi != pj {
			return pi < pj
		}
		return configs[i].Name < configs[j].Name
	})

	return configs
}

func kubeConfigPriority(c kubeConfig) int {
	if c.IsMonitoring {
		return 99
	}
	switch {
	case strings.Contains(c.Name, "test"):
		return 0
	case strings.Contains(c.Name, "preprod"):
		return 1
	case strings.Contains(c.Name, "prod"):
		return 2
	}
	return 10
}

// --- Fetch commands ---

func fetchPodsCmd(kubectl integration.KubectlService, configPath string) tea.Cmd {
	return func() tea.Msg {
		if configPath == "" {
			return podsLoadedMsg{err: fmt.Errorf("no KUBECONFIG selected")}
		}
		out, err := kubectl.GetResourceJSON(configPath, "pods")
		if err != nil {
			return podsLoadedMsg{err: err}
		}
		pods, err := parsePods(out)
		if err != nil {
			return podsLoadedMsg{err: err}
		}
		return podsLoadedMsg{pods: pods}
	}
}

func fetchServicesCmd(kubectl integration.KubectlService, configPath string) tea.Cmd {
	return func() tea.Msg {
		if configPath == "" {
			return servicesLoadedMsg{err: fmt.Errorf("no KUBECONFIG selected")}
		}
		out, err := kubectl.GetResourceJSON(configPath, "services")
		if err != nil {
			return servicesLoadedMsg{err: err}
		}
		svcs, err := parseServices(out)
		if err != nil {
			return servicesLoadedMsg{err: err}
		}
		return servicesLoadedMsg{services: svcs}
	}
}

func fetchDeploymentsCmd(kubectl integration.KubectlService, configPath string) tea.Cmd {
	return func() tea.Msg {
		if configPath == "" {
			return deploymentsLoadedMsg{err: fmt.Errorf("no KUBECONFIG selected")}
		}
		out, err := kubectl.GetResourceJSON(configPath, "deployments")
		if err != nil {
			return deploymentsLoadedMsg{err: err}
		}
		deps, err := parseDeployments(out)
		if err != nil {
			return deploymentsLoadedMsg{err: err}
		}
		return deploymentsLoadedMsg{deployments: deps}
	}
}

func scaleDeploymentCmd(kubectl integration.KubectlService, name string, replicas int, configPath string) tea.Cmd {
	return func() tea.Msg {
		if err := kubectl.ScaleDeployment(configPath, name, replicas); err != nil {
			return errMsg{source: "kube", err: err}
		}
		return kubeActionDoneMsg{message: fmt.Sprintf("Scaled %s → %d replicas", name, replicas)}
	}
}

// kubeCmdOutputCmd runs kubectl and returns an OpenModalMsg with the output.
func kubeCmdOutputCmd(kubectl integration.KubectlService, title string, configPath string, args ...string) tea.Cmd {
	return func() tea.Msg {
		out, _ := kubectl.RunCommand(configPath, args...)
		return OpenModalMsg{M: NewKubeOutputModal(title, string(out))}
	}
}

func kubeRefreshTickCmd() tea.Cmd {
	return tea.Every(10*time.Second, func(_ time.Time) tea.Msg { return kubeRefreshMsg{} })
}

// selectConfig switches KUBECONFIG and fetches all resource types.
func (s *KubeSection) selectConfig(cfg kubeConfig) tea.Cmd {
	s.activeConfig = cfg.Path
	os.Setenv("KUBECONFIG", cfg.Path) //nolint:errcheck
	s.pods = nil
	s.services = nil
	s.deployments = nil
	s.podErr = ""
	s.svcErr = ""
	s.depErr = ""
	s.loading = true
	s.svcLoading = true
	s.depLoading = true
	s.podCursor = 0
	s.svcCursor = 0
	s.depCursor = 0
	s.statusMsg = "Switched to " + cfg.Name
	kubectl := s.kubectl
	return tea.Batch(
		fetchPodsCmd(kubectl, cfg.Path),
		fetchServicesCmd(kubectl, cfg.Path),
		fetchDeploymentsCmd(kubectl, cfg.Path),
	)
}

// selectConfigByName returns the first non-monitoring config containing substr and its index.
func (s *KubeSection) selectConfigByName(substr string) (kubeConfig, int, bool) {
	for i, c := range s.configs {
		if strings.Contains(c.Name, substr) && !c.IsMonitoring {
			return c, i, true
		}
	}
	return kubeConfig{}, -1, false
}

// --- Update ---

func (s KubeSection) update(msg tea.Msg) (KubeSection, tea.Cmd) {
	switch msg := msg.(type) {
	case kubeConfigsLoadedMsg:
		s.configs = msg.configs
		s.loading = false
		if s.activeConfig == "" && len(msg.configs) > 0 {
			s.activeConfig = msg.configs[0].Path
			s.cursor = 0
		}
		if s.activeConfig != "" {
			s.loading = true
			s.svcLoading = true
			s.depLoading = true
			kubectl := s.kubectl
			return s, tea.Batch(
				fetchPodsCmd(kubectl, s.activeConfig),
				fetchServicesCmd(kubectl, s.activeConfig),
				fetchDeploymentsCmd(kubectl, s.activeConfig),
				kubeRefreshTickCmd(),
			)
		}
		return s, kubeRefreshTickCmd()

	case podsLoadedMsg:
		s.loading = false
		if msg.err != nil {
			s.podErr = msg.err.Error()
			s.pods = nil
		} else {
			s.pods = msg.pods
			s.podErr = ""
		}

	case servicesLoadedMsg:
		s.svcLoading = false
		if msg.err != nil {
			s.svcErr = msg.err.Error()
			s.services = nil
		} else {
			s.services = msg.services
			s.svcErr = ""
		}

	case deploymentsLoadedMsg:
		s.depLoading = false
		if msg.err != nil {
			s.depErr = msg.err.Error()
			s.deployments = nil
		} else {
			s.deployments = msg.deployments
			s.depErr = ""
		}

	case kubeRefreshMsg:
		// Background refresh: don't clear old data — only set loading when there's no data yet.
		if s.activeConfig != "" {
			if len(s.pods) == 0 {
				s.loading = true
			}
			if len(s.services) == 0 {
				s.svcLoading = true
			}
			if len(s.deployments) == 0 {
				s.depLoading = true
			}
			kubectl := s.kubectl
			return s, tea.Batch(
				fetchPodsCmd(kubectl, s.activeConfig),
				fetchServicesCmd(kubectl, s.activeConfig),
				fetchDeploymentsCmd(kubectl, s.activeConfig),
				kubeRefreshTickCmd(),
			)
		}
		return s, kubeRefreshTickCmd()

	case kubeActionDoneMsg:
		s.statusMsg = msg.message
		if s.activeConfig != "" {
			return s, fetchDeploymentsCmd(s.kubectl, s.activeConfig)
		}

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

func (s KubeSection) handleKey(msg tea.KeyMsg) (KubeSection, tea.Cmd) {
	switch msg.String() {
	case "tab":
		// Toggle between the config table (upper) and the last active resource pane (lower).
		if s.activePane == kubePaneConfigs {
			s.activePane = s.lastResourcePane
		} else {
			s.activePane = kubePaneConfigs
		}

	case "o":
		// Switch bottom tab to Pods and move focus there.
		s.lastResourcePane = kubePanePods
		s.activePane = kubePanePods

	case "e":
		// Switch bottom tab to Services and move focus there.
		s.lastResourcePane = kubePaneServices
		s.activePane = kubePaneServices

	case "j", "down":
		switch s.activePane {
		case kubePaneConfigs:
			if s.cursor < len(s.configs)-1 {
				s.cursor++
			}
		case kubePanePods:
			if s.podCursor < len(s.pods)-1 {
				s.podCursor++
			}
		case kubePaneServices:
			if s.svcCursor < len(s.services)-1 {
				s.svcCursor++
			}
		case kubePaneDeployments:
			if s.depCursor < len(s.deployments)-1 {
				s.depCursor++
			}
		}

	case "k", "up":
		switch s.activePane {
		case kubePaneConfigs:
			if s.cursor > 0 {
				s.cursor--
			}
		case kubePanePods:
			if s.podCursor > 0 {
				s.podCursor--
			}
		case kubePaneServices:
			if s.svcCursor > 0 {
				s.svcCursor--
			}
		case kubePaneDeployments:
			if s.depCursor > 0 {
				s.depCursor--
			}
		}

	case "enter":
		if s.activePane == kubePaneConfigs && len(s.configs) > 0 && s.cursor < len(s.configs) {
			c := s.configs[s.cursor]
			return s, s.selectConfig(c)
		}

	case "t":
		if c, idx, ok := s.selectConfigByName("test"); ok {
			s.cursor = idx
			s.activePane = kubePaneConfigs
			return s, s.selectConfig(c)
		}
		s.statusMsg = "No test config found"

	case "s":
		// Switch to staging/preprod config.
		if c, idx, ok := s.selectConfigByName("preprod"); ok {
			s.cursor = idx
			s.activePane = kubePaneConfigs
			return s, s.selectConfig(c)
		}
		s.statusMsg = "No staging (preprod) config found"

	case "p":
		// Switch to production config (not preprod).
		for i, cc := range s.configs {
			if strings.Contains(cc.Name, "prod") && !strings.Contains(cc.Name, "preprod") && !cc.IsMonitoring {
				s.cursor = i
				s.activePane = kubePaneConfigs
				return s, s.selectConfig(cc)
			}
		}
		s.statusMsg = "No production config found"

	// Pod / resource hotkeys
	case "l":
		// l = logs when focused in pods pane
		if s.activePane == kubePanePods && len(s.pods) > 0 && s.podCursor < len(s.pods) {
			pod := s.pods[s.podCursor].Name
			return s, kubeCmdOutputCmd(s.kubectl, "Logs: "+pod, s.activeConfig, "logs", pod, "--tail=200")
		}
		// no longer switches to deployments tab

	case "m":
		// m = switch to Deployments tab
		s.lastResourcePane = kubePaneDeployments
		s.activePane = kubePaneDeployments

	case "d":
		switch s.activePane {
		case kubePanePods:
			if len(s.pods) > 0 && s.podCursor < len(s.pods) {
				pod := s.pods[s.podCursor].Name
				return s, kubeCmdOutputCmd(s.kubectl, "Describe pod: "+pod, s.activeConfig, "describe", "pod", pod)
			}
		case kubePaneServices:
			if len(s.services) > 0 && s.svcCursor < len(s.services) {
				svc := s.services[s.svcCursor].Name
				return s, kubeCmdOutputCmd(s.kubectl, "Describe svc: "+svc, s.activeConfig, "describe", "service", svc)
			}
		case kubePaneDeployments:
			if len(s.deployments) > 0 && s.depCursor < len(s.deployments) {
				dep := s.deployments[s.depCursor].Name
				return s, kubeCmdOutputCmd(s.kubectl, "Describe deploy: "+dep, s.activeConfig, "describe", "deployment", dep)
			}
		}

	case "y":
		switch s.activePane {
		case kubePanePods:
			if len(s.pods) > 0 && s.podCursor < len(s.pods) {
				pod := s.pods[s.podCursor].Name
				return s, kubeCmdOutputCmd(s.kubectl, "YAML: "+pod, s.activeConfig, "get", "pod", pod, "-o", "yaml")
			}
		case kubePaneServices:
			if len(s.services) > 0 && s.svcCursor < len(s.services) {
				svc := s.services[s.svcCursor].Name
				return s, kubeCmdOutputCmd(s.kubectl, "YAML: "+svc, s.activeConfig, "get", "service", svc, "-o", "yaml")
			}
		case kubePaneDeployments:
			if len(s.deployments) > 0 && s.depCursor < len(s.deployments) {
				dep := s.deployments[s.depCursor].Name
				return s, kubeCmdOutputCmd(s.kubectl, "YAML: "+dep, s.activeConfig, "get", "deployment", dep, "-o", "yaml")
			}
		}

	case "r":
		// 'r' scales replicas on the deployments pane.
		if s.activePane == kubePaneDeployments && len(s.deployments) > 0 && s.depCursor < len(s.deployments) {
			dep := s.deployments[s.depCursor]
			depName := dep.Name
			cfgPath := s.activeConfig
			kubectl := s.kubectl
			return s, func() tea.Msg {
				return OpenModalMsg{M: NewInputModal(
					"Scale deployment: "+depName,
					"number of replicas",
					func(value string) tea.Cmd {
						n, err := strconv.Atoi(strings.TrimSpace(value))
						if err != nil || n < 0 {
							return func() tea.Msg {
								return errMsg{source: "kube", err: fmt.Errorf("invalid replica count %q — enter a non-negative integer", value)}
							}
						}
						return func() tea.Msg {
							return OpenModalMsg{M: ConfirmModal{
								message:   fmt.Sprintf("Scale %s to %d replicas?", depName, n),
								onConfirm: scaleDeploymentCmd(kubectl, depName, n, cfgPath),
							}}
						}
					},
				)}
			}
		}

	case "right":
		cmds := s.buildCommandPalette()
		if len(cmds) > 0 {
			configRows := len(s.configs) + 3
			maxConfig := s.termHeight / 3
			if configRows > maxConfig {
				configRows = maxConfig
			}
			var anchorY int
			switch s.activePane {
			case kubePaneConfigs:
				anchorY = 4 + s.cursor
			case kubePanePods:
				anchorY = configRows + 6 + s.podCursor
			case kubePaneServices:
				anchorY = configRows + 6 + s.svcCursor
			case kubePaneDeployments:
				anchorY = configRows + 6 + s.depCursor
			}
			return s, func() tea.Msg {
				return OpenModalMsg{M: CommandPaletteModal{commands: cmds, AnchorY: anchorY}}
			}
		}
	}
	return s, nil
}

// buildCommandPalette returns context-sensitive actions for the active Kube pane.
func (s KubeSection) buildCommandPalette() []CommandEntry {
	kubectl := s.kubectl
	cfg := s.activeConfig

	switch s.activePane {
	case kubePaneConfigs:
		var entries []CommandEntry
		if len(s.configs) > 0 && s.cursor < len(s.configs) {
			c := s.configs[s.cursor]
			entries = append(entries, CommandEntry{Key: "enter", Desc: "select", Cmd: s.selectConfig(c)})
		}
		if c, _, ok := s.selectConfigByName("test"); ok {
			entries = append(entries, CommandEntry{Key: "t", Desc: "test env", Cmd: s.selectConfig(c)})
		}
		if c, _, ok := s.selectConfigByName("preprod"); ok {
			entries = append(entries, CommandEntry{Key: "s", Desc: "staging env", Cmd: s.selectConfig(c)})
		}
		for _, cc := range s.configs {
			if strings.Contains(cc.Name, "prod") && !strings.Contains(cc.Name, "preprod") && !cc.IsMonitoring {
				entries = append(entries, CommandEntry{Key: "p", Desc: "prod env", Cmd: s.selectConfig(cc)})
				break
			}
		}
		return entries

	case kubePanePods:
		if len(s.pods) == 0 || s.podCursor >= len(s.pods) {
			return nil
		}
		pod := s.pods[s.podCursor].Name
		return []CommandEntry{
			{Key: "l", Desc: "logs", Cmd: kubeCmdOutputCmd(kubectl, "Logs: "+pod, cfg, "logs", pod, "--tail=200")},
			{Key: "d", Desc: "describe", Cmd: kubeCmdOutputCmd(kubectl, "Describe pod: "+pod, cfg, "describe", "pod", pod)},
			{Key: "y", Desc: "yaml", Cmd: kubeCmdOutputCmd(kubectl, "YAML: "+pod, cfg, "get", "pod", pod, "-o", "yaml")},
		}

	case kubePaneServices:
		if len(s.services) == 0 || s.svcCursor >= len(s.services) {
			return nil
		}
		svc := s.services[s.svcCursor].Name
		return []CommandEntry{
			{Key: "d", Desc: "describe", Cmd: kubeCmdOutputCmd(kubectl, "Describe svc: "+svc, cfg, "describe", "service", svc)},
			{Key: "y", Desc: "yaml", Cmd: kubeCmdOutputCmd(kubectl, "YAML: "+svc, cfg, "get", "service", svc, "-o", "yaml")},
		}

	case kubePaneDeployments:
		if len(s.deployments) == 0 || s.depCursor >= len(s.deployments) {
			return nil
		}
		dep := s.deployments[s.depCursor]
		depName := dep.Name
		cfgPath := cfg
		return []CommandEntry{
			{Key: "r", Desc: "scale", Cmd: func() tea.Msg {
				return OpenModalMsg{M: NewInputModal(
					"Scale deployment: "+depName,
					"number of replicas",
					func(value string) tea.Cmd {
						n, err := strconv.Atoi(strings.TrimSpace(value))
						if err != nil || n < 0 {
							return func() tea.Msg {
								return errMsg{source: "kube", err: fmt.Errorf("invalid replica count %q — enter a non-negative integer", value)}
							}
						}
						return func() tea.Msg {
							return OpenModalMsg{M: ConfirmModal{
								message:   fmt.Sprintf("Scale %s to %d replicas?", depName, n),
								onConfirm: scaleDeploymentCmd(kubectl, depName, n, cfgPath),
							}}
						}
					},
				)}
			}},
			{Key: "d", Desc: "describe", Cmd: kubeCmdOutputCmd(kubectl, "Describe deploy: "+depName, cfg, "describe", "deployment", depName)},
			{Key: "y", Desc: "yaml", Cmd: kubeCmdOutputCmd(kubectl, "YAML: "+depName, cfg, "get", "deployment", depName, "-o", "yaml")},
		}
	}
	return nil
}

// --- Parsers ---

func parsePods(data []byte) ([]kubePod, error) {
	var raw struct {
		Items []struct {
			Metadata struct {
				Name              string            `json:"name"`
				CreationTimestamp string            `json:"creationTimestamp"`
				Labels            map[string]string `json:"labels"`
				Annotations       map[string]string `json:"annotations"`
			} `json:"metadata"`
			Spec struct {
				Containers []struct {
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
			Status struct {
				Phase             string `json:"phase"`
				ContainerStatuses []struct {
					Ready bool `json:"ready"`
					State struct {
						Waiting *struct {
							Reason string `json:"reason"`
						} `json:"waiting"`
						Terminated *struct {
							Reason string `json:"reason"`
						} `json:"terminated"`
					} `json:"state"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse pods: %w", err)
	}

	now := time.Now()
	var pods []kubePod
	for _, item := range raw.Items {
		total := len(item.Status.ContainerStatuses)
		ready := 0
		failReason := ""
		for _, cs := range item.Status.ContainerStatuses {
			if cs.Ready {
				ready++
			}
			if failReason == "" {
				if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
					failReason = cs.State.Waiting.Reason
				} else if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
					failReason = cs.State.Terminated.Reason
				}
			}
		}

		// Version priority: annotations["rollme"] > labels["app.kubernetes.io/version"] > labels["rollme"] > image tag
		version := item.Metadata.Annotations["rollme"]
		if version == "" {
			version = item.Metadata.Labels["app.kubernetes.io/version"]
		}
		if version == "" {
			version = item.Metadata.Labels["rollme"]
		}
		if version == "" && len(item.Spec.Containers) > 0 {
			img := item.Spec.Containers[0].Image
			if idx := strings.LastIndex(img, ":"); idx >= 0 {
				version = img[idx+1:]
			}
		}
		if version == "" {
			version = "-"
		}

		// Status: use container fail reason when available, else phase
		status := item.Status.Phase
		if failReason != "" {
			status = failReason
		}

		pods = append(pods, kubePod{
			Name:    item.Metadata.Name,
			Ready:   fmt.Sprintf("%d/%d", ready, total),
			Version: version,
			Status:  status,
			Age:     kubeAge(item.Metadata.CreationTimestamp, now),
		})
	}
	return pods, nil
}

func parseServices(data []byte) ([]kubeService, error) {
	var raw struct {
		Items []struct {
			Metadata struct {
				Name              string `json:"name"`
				CreationTimestamp string `json:"creationTimestamp"`
			} `json:"metadata"`
			Spec struct {
				Type      string `json:"type"`
				ClusterIP string `json:"clusterIP"`
				Ports     []struct {
					Port     int    `json:"port"`
					Protocol string `json:"protocol"`
				} `json:"ports"`
			} `json:"spec"`
			Status struct {
				LoadBalancer struct {
					Ingress []struct {
						IP       string `json:"ip"`
						Hostname string `json:"hostname"`
					} `json:"ingress"`
				} `json:"loadBalancer"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse services: %w", err)
	}

	now := time.Now()
	var svcs []kubeService
	for _, item := range raw.Items {
		extIP := "<none>"
		if len(item.Status.LoadBalancer.Ingress) > 0 {
			ing := item.Status.LoadBalancer.Ingress[0]
			if ing.IP != "" {
				extIP = ing.IP
			} else if ing.Hostname != "" {
				extIP = ing.Hostname
			}
		}

		var ports []string
		for _, p := range item.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
		portsStr := strings.Join(ports, ",")
		if portsStr == "" {
			portsStr = "-"
		}

		svcs = append(svcs, kubeService{
			Name:       item.Metadata.Name,
			Type:       item.Spec.Type,
			ClusterIP:  item.Spec.ClusterIP,
			ExternalIP: extIP,
			Ports:      portsStr,
			Age:        kubeAge(item.Metadata.CreationTimestamp, now),
		})
	}
	return svcs, nil
}

func parseDeployments(data []byte) ([]kubeDeployment, error) {
	var raw struct {
		Items []struct {
			Metadata struct {
				Name              string `json:"name"`
				CreationTimestamp string `json:"creationTimestamp"`
			} `json:"metadata"`
			Status struct {
				Replicas          int `json:"replicas"`
				ReadyReplicas     int `json:"readyReplicas"`
				UpdatedReplicas   int `json:"updatedReplicas"`
				AvailableReplicas int `json:"availableReplicas"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse deployments: %w", err)
	}

	now := time.Now()
	var deps []kubeDeployment
	for _, item := range raw.Items {
		deps = append(deps, kubeDeployment{
			Name:      item.Metadata.Name,
			Ready:     fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, item.Status.Replicas),
			UpToDate:  strconv.Itoa(item.Status.UpdatedReplicas),
			Available: strconv.Itoa(item.Status.AvailableReplicas),
			Age:       kubeAge(item.Metadata.CreationTimestamp, now),
		})
	}
	return deps, nil
}

func kubeAge(ts string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

// --- View ---

func (s KubeSection) view(width, height int) string {
	configRows := len(s.configs) + 3
	maxConfigRows := height / 3
	if configRows > maxConfigRows {
		configRows = maxConfigRows
	}
	if configRows < 4 {
		configRows = 4
	}
	// 2 tooltip lines under config (legend + hotkeys) + sep + paneBar + paneSep + 2 tooltip lines under resource (legend + hotkeys)
	resourceH := height - configRows - 8
	if resourceH < 4 {
		resourceH = 4
	}

	configView := s.viewConfigTable(width, configRows)
	configLegend := KubeConfigLegendBar(width)
	configHints := buildTooltipLine("enter:select  t:test  s:staging/preprod  p:prod  →:commands", width)
	sep := sectionSepLine(width)
	paneSep := sectionSepLine(width)
	paneBar := s.viewPaneBar(width)
	resourceView := s.viewResourcePane(width, resourceH)
	resourceLegend := s.activeResourceLegendBar(width)
	resourceHints := buildTooltipLine("tab:focus  l:logs  y:yaml  d:describe  →:commands", width)

	return strings.Join([]string{
		configView, configLegend, configHints,
		sep,
		paneBar, paneSep,
		resourceView, resourceLegend, resourceHints,
	}, "\n")
}

// buildTooltipLine renders a single-line muted tooltip, truncated to width.
func buildTooltipLine(text string, width int) string {
	s := lipgloss.NewStyle().Foreground(colorMuted)
	t := truncStr(text, width)
	return s.Render("  " + t)
}

// activeResourceLegendBar returns the legend bar appropriate for the active bottom pane.
func (s KubeSection) activeResourceLegendBar(width int) string {
	switch s.lastResourcePane {
	case kubePanePods:
		return PodStatusLegendBar(width)
	default:
		return ""
	}
}

func (s KubeSection) viewPaneBar(width int) string {
	type tabDef struct {
		pane  kubePane
		label string
	}
	tabs := []tabDef{
		{kubePanePods, "[o] Pods"},
		{kubePaneServices, "[e] Services"},
		{kubePaneDeployments, "[m] Deployments"},
	}
	// Match the exact rendered widths of the first 3 top app tabs so the two
	// tab bars are pixel-perfect aligned.
	topWidths := [3]int{
		lipgloss.Width(tabInactiveStyle.Render(tabLabels[0])),
		lipgloss.Width(tabInactiveStyle.Render(tabLabels[1])),
		lipgloss.Width(tabInactiveStyle.Render(tabLabels[2])),
	}
	var parts []string
	for i, t := range tabs {
		w := topWidths[i]
		if t.pane == s.lastResourcePane {
			if s.activePane == kubePaneConfigs {
				parts = append(parts, tabDimStyle.Width(w).Render(t.label))
			} else {
				parts = append(parts, tabActiveStyle.Width(w).Render(t.label))
			}
		} else {
			parts = append(parts, tabInactiveStyle.Width(w).Render(t.label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (s KubeSection) viewConfigTable(width, height int) string {
	nameW := 24
	pathW := width - nameW - 12
	if pathW < 20 {
		pathW = 20
	}

	colH := func(str string, w int) string {
		return columnHeaderStyle.Width(w).Render(truncStr(str, w))
	}
	header := colH("ENV", nameW) + " " + colH("CONFIG PATH", pathW) + " " + colH("", 8)

	var lines []string
	lines = append(lines, header, tableHeaderSepLine(width))

	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.cursor, len(s.configs), maxRows)

	for i := start; i < end; i++ {
		c := s.configs[i]
		badge := ""
		if c.Path == s.activeConfig {
			badge = lipgloss.NewStyle().Foreground(colorGreen).Render("[ACTIVE]")
		}
		line := fmt.Sprintf("%-*s %-*s %s",
			nameW, truncStr(c.Name, nameW),
			pathW, truncStr(c.Path, pathW),
			badge,
		)
		isCursorActive := i == s.cursor && s.activePane == kubePaneConfigs
		isCursorDim := i == s.cursor && s.activePane != kubePaneConfigs
		cfgCol := colorFg
		if c.Path == s.activeConfig {
			cfgCol = colorGreen
		} else if c.IsMonitoring {
			cfgCol = colorMuted
		}
		switch {
		case isCursorActive:
			lines = append(lines, selectedRowStyle.Foreground(cfgCol).Width(width).Render(line))
		case isCursorDim:
			lines = append(lines, selectedRowDimStyle.Foreground(cfgCol).Width(width).Render(line))
		case c.Path == s.activeConfig:
			lines = append(lines, lipgloss.NewStyle().Foreground(colorGreen).Width(width).Render(line))
		case c.IsMonitoring:
			lines = append(lines, lipgloss.NewStyle().Foreground(colorMuted).Width(width).Render(line))
		default:
			lines = append(lines, normalRowStyle.Width(width).Render(line))
		}
	}
	lines = append(lines, tableHeaderSepLine(width))

	return strings.Join(lines, "\n")
}

func (s KubeSection) viewResourcePane(width, height int) string {
	// Always render the last-selected resource tab, regardless of which section has focus.
	switch s.lastResourcePane {
	case kubePanePods:
		return s.viewPodsTable(width, height)
	case kubePaneServices:
		return s.viewServicesTable(width, height)
	case kubePaneDeployments:
		return s.viewDeploymentsTable(width, height)
	}
	return ""
}

func (s KubeSection) viewPodsTable(width, height int) string {
	if p := subPanePlaceholder(s.loading, len(s.pods), "Loading pods…", s.podErr, "No pods found in current namespace"); p != "" {
		return p
	}

	nameW := width - 7 - 15 - 13 - 7 - 5
	if nameW < 20 {
		nameW = 20
	}

	colH := func(str string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(str, w)) }
	header := colH("POD", nameW) + " " + colH("READY", 6) + " " + colH("VERSION", 14) + " " + colH("STATUS", 12) + " " + colH("AGE", 6)

	var lines []string
	lines = append(lines, header, tableHeaderSepLine(width))

	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.podCursor, len(s.pods), maxRows)

	for i := start; i < end; i++ {
		p := s.pods[i]
		line := fmt.Sprintf("%-*s %-6s %-14s %-12s %-6s",
			nameW, truncStr(p.Name, nameW),
			truncStr(p.Ready, 6),
			truncStr(p.Version, 14),
			truncStr(p.Status, 12),
			truncStr(p.Age, 6),
		)
		col := podStatusColor(p.Status)
		if i == s.podCursor && s.activePane == kubePanePods {
			lines = append(lines, selectedRowStyle.Foreground(col).Width(width).Render(line))
		} else if i == s.podCursor {
			lines = append(lines, selectedRowDimStyle.Foreground(col).Width(width).Render(line))
		} else {
			lines = append(lines, lipgloss.NewStyle().Foreground(col).Width(width).Render(line))
		}
	}
	lines = append(lines, tableHeaderSepLine(width))

	return strings.Join(lines, "\n")
}

func (s KubeSection) viewServicesTable(width, height int) string {
	if p := subPanePlaceholder(s.svcLoading, len(s.services), "Loading services…", s.svcErr, "No services found"); p != "" {
		return p
	}

	typeW := 12
	clusterIPW := 16
	extIPW := 16
	portsW := 20
	ageW := 6
	nameW := width - typeW - clusterIPW - extIPW - portsW - ageW - 10
	if nameW < 16 {
		nameW = 16
	}

	colH := func(str string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(str, w)) }
	header := colH("NAME", nameW) + " " + colH("TYPE", typeW) + " " + colH("CLUSTER-IP", clusterIPW) +
		" " + colH("EXTERNAL-IP", extIPW) + " " + colH("PORT(S)", portsW) + " " + colH("AGE", ageW)

	var lines []string
	lines = append(lines, header, tableHeaderSepLine(width))

	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.svcCursor, len(s.services), maxRows)

	for i := start; i < end; i++ {
		svc := s.services[i]
		line := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s",
			nameW, truncStr(svc.Name, nameW),
			typeW, truncStr(svc.Type, typeW),
			clusterIPW, truncStr(svc.ClusterIP, clusterIPW),
			extIPW, truncStr(svc.ExternalIP, extIPW),
			portsW, truncStr(svc.Ports, portsW),
			ageW, truncStr(svc.Age, ageW),
		)
		if i == s.svcCursor && s.activePane == kubePaneServices {
			lines = append(lines, selectedRowStyle.Foreground(colorFg).Width(width).Render(line))
		} else if i == s.svcCursor {
			lines = append(lines, selectedRowDimStyle.Foreground(colorFg).Width(width).Render(line))
		} else {
			lines = append(lines, normalRowStyle.Width(width).Render(line))
		}
	}
	lines = append(lines, tableHeaderSepLine(width))

	return strings.Join(lines, "\n")
}

func (s KubeSection) viewDeploymentsTable(width, height int) string {
	if p := subPanePlaceholder(s.depLoading, len(s.deployments), "Loading deployments…", s.depErr, "No deployments found"); p != "" {
		return p
	}

	readyW := 8
	upToDateW := 10
	availW := 10
	ageW := 6
	nameW := width - readyW - upToDateW - availW - ageW - 8
	if nameW < 20 {
		nameW = 20
	}

	colH := func(str string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(str, w)) }
	header := colH("NAME", nameW) + " " + colH("READY", readyW) + " " + colH("UP-TO-DATE", upToDateW) +
		" " + colH("AVAILABLE", availW) + " " + colH("AGE", ageW)

	var lines []string
	lines = append(lines, header, tableHeaderSepLine(width))

	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.depCursor, len(s.deployments), maxRows)

	for i := start; i < end; i++ {
		dep := s.deployments[i]
		line := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s",
			nameW, truncStr(dep.Name, nameW),
			readyW, truncStr(dep.Ready, readyW),
			upToDateW, truncStr(dep.UpToDate, upToDateW),
			availW, truncStr(dep.Available, availW),
			ageW, truncStr(dep.Age, ageW),
		)
		if i == s.depCursor && s.activePane == kubePaneDeployments {
			lines = append(lines, selectedRowStyle.Foreground(colorFg).Width(width).Render(line))
		} else if i == s.depCursor {
			lines = append(lines, selectedRowDimStyle.Foreground(colorFg).Width(width).Render(line))
		} else {
			lines = append(lines, normalRowStyle.Width(width).Render(line))
		}
	}
	lines = append(lines, tableHeaderSepLine(width))

	return strings.Join(lines, "\n")
}

func podStatusColor(status string) lipgloss.Color {
	lower := strings.ToLower(status)
	switch lower {
	case "running":
		return colorGreen
	case "completed", "succeeded":
		return colorSubtle
	case "pending", "containercreating", "podinitialing", "init:0/1", "init:0/2",
		"terminating", "podinitializing":
		return colorYellow
	case "failed", "crashloopbackoff", "error", "oomkilled", "imagepullbackoff",
		"errimagepull", "invalidimagename", "createcontainererror",
		"createcontainerconfigerror", "runiniterror":
		return colorRed
	}
	// Heuristic: any status containing "error" or "backoff" is a failure.
	if strings.Contains(lower, "error") || strings.Contains(lower, "backoff") ||
		strings.Contains(lower, "fail") || strings.Contains(lower, "oom") {
		return colorRed
	}
	// Heuristic: any status containing "init" or "creating" is starting.
	if strings.Contains(lower, "init") || strings.Contains(lower, "creating") {
		return colorYellow
	}
	return colorFg
}

// scrollWindow returns start/end indices for a scrolling list view.
func scrollWindow(cursor, total, maxRows int) (int, int) {
	start := 0
	if cursor >= maxRows {
		start = cursor - maxRows + 1
	}
	end := start + maxRows
	if end > total {
		end = total
	}
	return start, end
}

func (s KubeSection) helpKeys() []HelpEntry { return KubeKeys }
