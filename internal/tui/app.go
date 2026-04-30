package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/background"
	"github.com/andob/jirlab/internal/debug"
	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

// Tab identifies one of the five main sections.
type Tab int

const (
	TabBoard   Tab = 0
	TabRepos   Tab = 1
	TabMRs     Tab = 2
	TabKube    Tab = 3
	TabChats   Tab = 4
)

var tabLabels = [5]string{
	"[1] Sprint Board",
	"[2] Repositories",
	"[3] Merge Requests",
	"[4] Kubernetes",
	"[5] Chats",
}

// switchTabMsg is sent internally to switch the active tab.
type switchTabMsg Tab

// errMsg carries a network or API error back to the root model.
type errMsg struct {
	source string
	err    error
}

// spinTickMsg is sent on each 100 ms frame while a refresh is in progress,
// and once more (after a 2 s delay) to reset the [0] tab colour on completion.
type spinTickMsg struct{}

// spinFrames is the braille spinner animation sequence.
var spinFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// spinTickCmd schedules the next 100 ms spinner frame.
func spinTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg { return spinTickMsg{} })
}

// AppModel is the root Bubble Tea model for the lazygit-style TUI.
type AppModel struct {
	activeTab Tab

	board   BoardSection
	repos   ReposSection
	mrs     MRsSection
	kube    KubeSection
	tracker TrackerSection // holds worklog state for Board's l key
	chats   ChatsSection

	activeModal modal

	width    int
	height   int
	errorMsg string // shown in the error banner; cleared by esc
	debug    bool

	// Refresh-state tracking for the [0] tab animation.
	refreshGeneration uint64    // incremented on each 0 press to invalidate stale worker ticks
	refreshPending    int       // number of outstanding "core" fetch results expected
	refreshFailed     bool      // any fetch in the current batch returned an error
	refreshDoneAt     time.Time // when refreshPending last reached zero
	spinFrame         int       // current braille-spinner frame index

	// Services
	jira         integration.JiraService
	gitlab       integration.GitLabService
	gitlabClient service.GitLabClient
	gitlabAPIURL string
	shell        integration.ShellService
}

// NewAppModel creates the root model.
func NewAppModel(
	jira integration.JiraService,
	gitlab integration.GitLabService,
	gitlabClient service.GitLabClient,
	boardID, myUserKey, gitlabAPIURL, jiraBaseURL string,
	azureClientID string,
	debugMode bool,
) AppModel {
	shell := integration.NewShellService()
	var msgraph integration.MSGraphService
	if azureClientID != "" {
		msgraph = integration.NewMSGraphClient(azureClientID)
	}
	return AppModel{
		activeTab:    TabBoard,
		board:        newBoardSection(jira, gitlab, integration.NewGitCmdService(), shell, boardID, myUserKey, jiraBaseURL),
		repos:        newReposSection(integration.NewGitCmdService(), shell, gitlab),
		mrs:          newMRsSection(gitlab, integration.NewGitCmdService(), integration.NewFilesystemService(), shell),
		kube:         newKubeSection(integration.NewKubectlService()),
		tracker:      newTrackerSection(jira, myUserKey, shell),
		chats:        NewChatsSection(msgraph, shell),
		jira:         jira,
		gitlab:       gitlab,
		gitlabClient: gitlabClient,
		gitlabAPIURL: gitlabAPIURL,
		debug:        debugMode,
		shell:        shell,
	}
}

// Init fires startup commands.
func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		ScanReposCmd(),
		background.Worker(60*time.Second, m.refreshGeneration),
		kubeInitCmd(),
	}
	if m.board.jira != nil && m.board.boardID != "" {
		cmds = append(cmds, fetchBoardIssuesCmd(m.board.jira, m.board.boardID))
	}
	if m.mrs.gitlab != nil {
		cmds = append(cmds, fetchMRsCmd(m.mrs.gitlab, m.mrs.projectIDs))
	}
	if m.tracker.jira != nil {
		cmds = append(cmds, fetchWorklogsCmd(m.tracker.jira, m.tracker.accountID, m.tracker.currentDate))
	}
	if m.jira != nil {
		cmds = append(cmds, fetchMyAccountIDCmd(m.board.jira))
	}
	if cmd := m.chats.Init(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// buildLocalBranches extracts issue keys from repo branch names.
func buildLocalBranches(repos []service.Repo) (bools map[string]bool, names map[string]string) {
	bools = make(map[string]bool)
	names = make(map[string]string)
	for _, r := range repos {
		if r.CurrentBranch == "" {
			continue
		}
		matches := service.IssueKeyRe.FindStringSubmatch(r.CurrentBranch)
		if len(matches) >= 2 {
			key := strings.ToUpper(matches[1])
			bools[key] = true
			names[key] = r.CurrentBranch
		}
	}
	return
}

// buildRepoPaths maps issue keys to the repo path that has that branch checked out.
func buildRepoPaths(repos []service.Repo) map[string]string {
	paths := make(map[string]string)
	for _, r := range repos {
		if r.CurrentBranch == "" {
			continue
		}
		if m := service.IssueKeyRe.FindStringSubmatch(r.CurrentBranch); len(m) >= 2 {
			paths[strings.ToUpper(m[1])] = r.Path
		}
	}
	return paths
}

// decrementRefreshPending decrements the outstanding-fetch counter.
// When it reaches zero the refresh is considered complete; a 2 s cleanup tick
// is returned so the [0] tab colour resets after the result window expires.
func decrementRefreshPending(m AppModel, failed bool) (AppModel, tea.Cmd) {
	if failed {
		m.refreshFailed = true
	}
	if m.refreshPending > 0 {
		m.refreshPending--
		if m.refreshPending == 0 {
			m.refreshDoneAt = time.Now()
			// Schedule a redraw after 2 s to clear the success/failure colour.
			return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg { return spinTickMsg{} })
		}
	}
	return m, nil
}

// Update handles all incoming messages and delegates to the active section.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Always handle resize
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		m.board.termHeight = ws.Height
		m.repos.termHeight = ws.Height
		m.mrs.termHeight = ws.Height
		m.kube.termHeight = ws.Height
		return m, nil
	}

	// Network / API errors — display in error banner
	if em, ok := msg.(errMsg); ok {
		m.errorMsg = fmt.Sprintf("[%s] %s", em.source, em.err.Error())
		debug.Log("error from %s: %v", em.source, em.err)
		// Auto-copy error to clipboard for easier debugging/reporting.
		if m.shell != nil {
			return m, copyToClipboard(m.shell, m.errorMsg)
		}
		return m, nil
	}

	// Spinner tick — advance the animation frame while a refresh is pending.
	if _, ok := msg.(spinTickMsg); ok {
		if m.refreshPending > 0 {
			m.spinFrame = (m.spinFrame + 1) % len(spinFrames)
			return m, spinTickCmd()
		}
		// pending == 0: this is the 2 s cleanup tick — just trigger a redraw.
		return m, nil
	}

	// Background worker tick — trigger re-fetch of all data.
	if rm, ok := msg.(background.RefreshMsg); ok {
		if rm.Generation != m.refreshGeneration {
			// Stale tick from before the last 0 reset — ignore.
			return m, nil
		}
		var cmds []tea.Cmd
		pending := 0
		if m.board.jira != nil && m.board.boardID != "" {
			if len(m.board.allIssues) == 0 {
				m.board.loading = true
			}
			cmds = append(cmds, fetchBoardIssuesCmd(m.board.jira, m.board.boardID))
			pending++
		}
		if m.mrs.gitlab != nil {
			if len(m.mrs.mrs) == 0 {
				m.mrs.loading = true
			}
			cmds = append(cmds, fetchMRsCmd(m.mrs.gitlab, m.mrs.projectIDs))
		}
		if m.tracker.jira != nil {
			m.tracker.loading = true
			cmds = append(cmds, fetchWorklogsCmd(m.tracker.jira, m.tracker.accountID, m.tracker.currentDate))
			pending++
		}
		// Best-effort git fetch for all known repos to keep version tags current.
		if m.repos.git != nil {
			for _, repo := range m.repos.repos {
				if r := repo; r.Path != "" {
					git := m.repos.git
					cmds = append(cmds, func() tea.Msg {
						_ = git.FetchAll(r.Path)
						return nil
					})
				}
			}
		}
		if pending > 0 {
			m.refreshPending = pending
			m.refreshFailed = false
			cmds = append(cmds, spinTickCmd())
		}
		// Restart the one-shot worker with the same generation for the next cycle.
		cmds = append(cmds, background.Worker(60*time.Second, m.refreshGeneration))
		return m, tea.Batch(cmds...)
	}

	// Repos loaded → extract local branches for board coloring, then resolve GitLab IDs
	if rm, ok := msg.(reposLoadedMsg); ok {
		m.repos, _ = m.repos.update(rm)
		bools, names := buildLocalBranches(rm.repos)
		m.board.localBranches = bools
		m.board.branchNames = names
		m.board.repos = rm.repos
		m.board.repoPaths = buildRepoPaths(rm.repos)
		if m.gitlabClient != nil && m.gitlabAPIURL != "" {
			return m, resolveRepoProjectsCmd(rm.repos, m.gitlabClient, m.gitlabAPIURL)
		}
		// No GitLab client: repos scan is the terminal repos event — decrement pending.
		var cleanupCmd tea.Cmd
		m, cleanupCmd = decrementRefreshPending(m, false)
		return m, cleanupCmd
	}

	// Repo action done (navigate, checkout) — route to repos
	if ra, ok := msg.(repoActionDoneMsg); ok {
		var cmd tea.Cmd
		m.repos, cmd = m.repos.update(ra)
		return m, cmd
	}

	// Repos subtab data messages — always routed to repos section
	if _, ok := msg.(repoBranchesLoadedMsg); ok {
		var cmd tea.Cmd
		m.repos, cmd = m.repos.update(msg)
		return m, cmd
	}
	if _, ok := msg.(repoTagsLoadedMsg); ok {
		var cmd tea.Cmd
		m.repos, cmd = m.repos.update(msg)
		return m, cmd
	}
	if _, ok := msg.(repoSubMRsLoadedMsg); ok {
		var cmd tea.Cmd
		m.repos, cmd = m.repos.update(msg)
		return m, cmd
	}
	if _, ok := msg.(repoPipelinesLoadedMsg); ok {
		var cmd tea.Cmd
		m.repos, cmd = m.repos.update(msg)
		return m, cmd
	}
	if _, ok := msg.(repoLatestTagMsg); ok {
		var cmd tea.Cmd
		m.repos, cmd = m.repos.update(msg)
		return m, cmd
	}
	if _, ok := msg.(repoCIVariablesLoadedMsg); ok {
		var cmd tea.Cmd
		m.repos, cmd = m.repos.update(msg)
		return m, cmd
	}

	// All-pipelines loaded — route to mrs section
	if _, ok := msg.(allPipelinesLoadedMsg); ok {
		var cmd tea.Cmd
		m.mrs, cmd = m.mrs.update(msg)
		return m, cmd
	}

	// Repos with projects resolved
	if rwp, ok := msg.(reposWithProjectsMsg); ok {
		var reposCmd tea.Cmd
		m.repos, reposCmd = m.repos.update(rwp)
		bools, names := buildLocalBranches(rwp.repos)
		m.board.localBranches = bools
		m.board.branchNames = names
		m.board.repos = rwp.repos
		m.board.repoPaths = buildRepoPaths(rwp.repos)
		m.mrs.repos = rwp.repos
		// Collect resolved project IDs and trigger a fresh MR fetch with all projects
		var pids []int
		repoNames := make(map[int]string)
		for _, r := range rwp.repos {
			if r.GitLabProjectID != 0 {
				pids = append(pids, r.GitLabProjectID)
				repoNames[r.GitLabProjectID] = r.Name
			}
		}
		m.mrs.projectIDs = pids
		m.mrs.repoNames = repoNames

		// Repos scan is now complete — decrement pending.
		m, cleanupCmd := decrementRefreshPending(m, false)

		// Fire best-effort latest-tag fetches for repos with a resolved version.
		var tagCmds []tea.Cmd
		for _, r := range rwp.repos {
			if r.Version != "" && r.GitLabProjectID != 0 {
				tagCmds = append(tagCmds, fetchRepoLatestTagCmd(m.repos.gitlab, r))
			}
		}

		if m.mrs.gitlab != nil {
			m.mrs.loading = true
			return m, tea.Batch(append([]tea.Cmd{reposCmd, cleanupCmd, fetchMRsCmd(m.mrs.gitlab, pids)}, tagCmds...)...)
		}
		return m, tea.Batch(append([]tea.Cmd{reposCmd, cleanupCmd}, tagCmds...)...)
	}

	// Board issues loaded (always route to board regardless of active tab)
	if bm, ok := msg.(boardIssuesLoadedMsg); ok {
		var cmd tea.Cmd
		m.board, cmd = m.board.update(bm)
		if bm.err == nil {
			keys := make(map[string]bool, len(bm.issues))
			for _, iss := range bm.issues {
				keys[iss.Key] = true
			}
			m.repos.sprintIssueKeys = keys
		}
		m, cleanupCmd := decrementRefreshPending(m, bm.err != nil)
		return m, tea.Batch(cmd, cleanupCmd)
	}

	// Board action done (assign, comment, etc.)
	if ba, ok := msg.(boardActionDoneMsg); ok {
		var cmd tea.Cmd
		m.board, cmd = m.board.update(ba)
		return m, cmd
	}

	// Issue details loaded → replace LoadingModal with DescriptionModal
	if idm, ok := msg.(issueDetailsLoadedMsg); ok {
		if idm.err != nil {
			m.activeModal = nil
			m.errorMsg = fmt.Sprintf("[issue] %s", idm.err.Error())
			debug.Log("issue details error: %v", idm.err)
		} else if idm.issue != nil {
			m.activeModal = DescriptionModal{Issue: *idm.issue}
		} else {
			m.activeModal = nil
		}
		return m, nil
	}

	// MRs loaded — update mrs section AND push MR statuses to board and repos
	if mm, ok := msg.(mrsLoadedMsg); ok {
		var cmd tea.Cmd
		m.mrs, cmd = m.mrs.update(mm)
		if mm.err == nil {
			statuses := BuildMRStatuses(mm.mrs)
			m.board.mrStatuses = statuses
			m.repos.mrStatuses = statuses
			m.repos.projectMRStatuses = BuildProjectMRStatuses(mm.mrs)
		}
		return m, cmd
	}

	// MR action done (merge/close) — route to mrs section
	if ma, ok := msg.(mrsActionDoneMsg); ok {
		var cmd tea.Cmd
		m.mrs, cmd = m.mrs.update(ma)
		return m, cmd
	}

	// Worklogs loaded
	if wm, ok := msg.(worklogsLoadedMsg); ok {
		var cmd tea.Cmd
		m.tracker, cmd = m.tracker.update(wm)
		m, cleanupCmd := decrementRefreshPending(m, wm.err != nil)
		return m, tea.Batch(cmd, cleanupCmd)
	}

	// Worklog added — route to tracker for status message + reload
	if wa, ok := msg.(worklogAddedMsg); ok {
		var cmd tea.Cmd
		m.tracker, cmd = m.tracker.update(wa)
		return m, cmd
	}

	// Notes messages — always routed to chats section regardless of active tab
	if ns, ok := msg.(noteSavedMsg); ok {
		var cmd tea.Cmd
		m.chats, cmd = m.chats.update(ns)
		return m, cmd
	}
	if nr, ok := msg.(noteRemovedMsg); ok {
		var cmd tea.Cmd
		m.chats, cmd = m.chats.update(nr)
		return m, cmd
	}

	// Templates messages — always routed to chats section regardless of active tab
	if ts, ok := msg.(templateSavedMsg); ok {
		var cmd tea.Cmd
		m.chats, cmd = m.chats.update(ts)
		return m, cmd
	}
	if tr, ok := msg.(templateRemovedMsg); ok {
		var cmd tea.Cmd
		m.chats, cmd = m.chats.update(tr)
		return m, cmd
	}

	// Chats messages — route to chats section
	if cl, ok := msg.(chatsLoadedMsg); ok {
		var cmd tea.Cmd
		m.chats, cmd = m.chats.update(cl)
		return m, cmd
	}
	if ca, ok := msg.(chatsAuthStartedMsg); ok {
		var cmd tea.Cmd
		m.chats, cmd = m.chats.update(ca)
		return m, cmd
	}
	if cd, ok := msg.(chatsAuthDoneMsg); ok {
		var cmd tea.Cmd
		m.chats, cmd = m.chats.update(cd)
		return m, cmd
	}

	// myAccountIDMsg — set board.myUserKey from /3/myself
	if mam, ok := msg.(myAccountIDMsg); ok {
		m.board.myUserKey = mam.id
		return m, nil
	}

	// Kube messages — always routed regardless of active tab
	for _, match := range []bool{
		func() bool { _, ok := msg.(kubeConfigsLoadedMsg); return ok }(),
		func() bool { _, ok := msg.(podsLoadedMsg); return ok }(),
		func() bool { _, ok := msg.(servicesLoadedMsg); return ok }(),
		func() bool { _, ok := msg.(deploymentsLoadedMsg); return ok }(),
		func() bool { _, ok := msg.(kubeRefreshMsg); return ok }(),
		func() bool { _, ok := msg.(kubeActionDoneMsg); return ok }(),
	} {
		if match {
			var cmd tea.Cmd
			m.kube, cmd = m.kube.update(msg)
			return m, cmd
		}
	}

	// If a modal is open, route to it first
	if m.activeModal != nil {
		if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" {
			m.activeModal = nil
			return m, nil
		}
		updated, cmd := m.activeModal.update(msg)
		m.activeModal = updated
		return m, cmd
	}

	// Dismiss error banner with esc
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" && m.errorMsg != "" {
		m.errorMsg = ""
		return m, nil
	}

	// Handle OpenModalMsg from sections
	if omm, ok := msg.(OpenModalMsg); ok {
		m.activeModal = omm.M
		return m, nil
	}

	// Handle tab-switch from sections
	if stm, ok := msg.(switchTabMsg); ok {
		m.activeTab = Tab(stm)
		return m, nil
	}

	// Keyboard — global keys
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			return m, tea.Quit
		case "1":
			m.activeTab = TabBoard
			return m, nil
		case "2":
			m.activeTab = TabRepos
			return m, nil
		case "3":
			m.activeTab = TabMRs
			return m, nil
		case "4":
			m.activeTab = TabKube
			return m, nil
		case "5":
			m.activeTab = TabChats
			return m, nil
		case "0":
			m.refreshGeneration++
			m.refreshFailed = false
			var cmds []tea.Cmd
			pending := 1 // repos scan always counts
			if m.board.jira != nil && m.board.boardID != "" {
				m.board.loading = true
				cmds = append(cmds, fetchBoardIssuesCmd(m.board.jira, m.board.boardID))
				pending++
			}
			if m.mrs.gitlab != nil {
				m.mrs.loading = true
				cmds = append(cmds, fetchMRsCmd(m.mrs.gitlab, m.mrs.projectIDs))
				// Refresh pipelines pane if it was previously loaded
				if len(m.mrs.allPipelines) > 0 || m.mrs.pipelinesLoading {
					m.mrs.pipelinesLoading = true
					cmds = append(cmds, fetchAllPipelinesCmd(m.mrs.gitlab, m.mrs.projectIDs, m.mrs.repoNames))
				}
			}
			if m.tracker.jira != nil {
				m.tracker.loading = true
				cmds = append(cmds, fetchWorklogsCmd(m.tracker.jira, m.tracker.accountID, m.tracker.currentDate))
				pending++
			}
			m.repos.loading = true
			m.repos.statusMsg = "Rescanning…"
			cmds = append(cmds, ScanReposCmd())
			// Refresh repos subtab if it was previously loaded
			if m.repos.subRepoPath != "" {
				if repo := m.repos.currentRepo(); repo != nil {
					switch m.repos.lastSubPane {
					case reposPaneBranches:
						m.repos.branchLoading = true
						cmds = append(cmds, fetchRepoBranchesCmd(m.repos.gitlab, *repo))
					case reposPaneTags:
						m.repos.tagLoading = true
						cmds = append(cmds, fetchRepoTagsCmd(m.repos.gitlab, *repo))
					case reposPaneMRs:
						m.repos.subMRLoading = true
						cmds = append(cmds, fetchRepoSubMRsCmd(m.repos.gitlab, *repo))
					case reposPanePipelines:
						m.repos.pipelineLoading = true
						cmds = append(cmds, fetchRepoPipelinesCmd(m.repos.gitlab, *repo))
					}
				}
			}
			if m.kube.activeConfig != "" {
				kubectl := m.kube.kubectl
				cfg := m.kube.activeConfig
				cmds = append(cmds, fetchPodsCmd(kubectl, cfg), fetchServicesCmd(kubectl, cfg), fetchDeploymentsCmd(kubectl, cfg))
			}
			m.refreshPending = pending
			// Reset the background timer so the next automatic refresh is 60 s from now.
			cmds = append(cmds, background.Worker(60*time.Second, m.refreshGeneration))
			cmds = append(cmds, spinTickCmd())
			return m, tea.Batch(cmds...)
		case "?":
			m.activeModal = HelpModal{
				sectionName: m.activeSectionName(),
				sectionKeys: m.activeSectionKeys(),
			}
			// Populate colour hint columns based on active section + bottom pane.
			if hm, ok := m.activeModal.(HelpModal); ok {
				switch m.activeTab {
				case TabBoard:
					hm.topColorEntries = issueColorEntries
					hm.topColorLabel = "Colours — issues"
				case TabRepos:
					hm.topColorEntries = repoColorEntries
					hm.topColorLabel = "Colours — repositories"
					switch m.repos.lastSubPane {
					case reposPaneBranches:
						hm.bottomColorEntries = branchColorEntries
						hm.bottomColorLabel = "Colours — branches"
					case reposPaneMRs:
						hm.bottomColorEntries = mrColorEntries
						hm.bottomColorLabel = "Colours — merge requests"
					case reposPanePipelines:
						hm.bottomColorEntries = pipelineStatusColorEntries
						hm.bottomColorLabel = "Colours — pipelines"
					}
				case TabMRs:
					hm.topColorEntries = mrColorEntries
					hm.topColorLabel = "Colours — merge requests"
					if len(m.mrs.allPipelines) > 0 || m.mrs.pipelinesLoading {
						hm.bottomColorEntries = pipelineStatusColorEntries
						hm.bottomColorLabel = "Colours — pipelines"
					}
				case TabKube:
					hm.topColorEntries = kubeConfigColorEntries
					hm.topColorLabel = "Colours — configs"
					switch m.kube.lastResourcePane {
					case kubePanePods:
						hm.bottomColorEntries = podStatusColorEntries
						hm.bottomColorLabel = "Colours — pods"
					case kubePaneServices:
						// services have no colour coding currently
					case kubePaneDeployments:
						// deployments have no colour coding currently
					}
				case TabChats:
					hm.topColorEntries = timeLogColorEntries
					hm.topColorLabel = "Colours — time logged"
					hm.bottomColorEntries = notePriorityColorEntries
					hm.bottomColorLabel = "Colours — note priority"
				}
				m.activeModal = hm
			}
			return m, nil
		}
	}

	// Delegate to active section
	return m.updateActiveSection(msg)
}

func (m AppModel) updateActiveSection(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.activeTab {
	case TabBoard:
		m.board, cmd = m.board.update(msg)
	case TabRepos:
		m.repos, cmd = m.repos.update(msg)
	case TabMRs:
		m.mrs, cmd = m.mrs.update(msg)
	case TabKube:
		m.kube, cmd = m.kube.update(msg)
	case TabChats:
		m.chats, cmd = m.chats.update(msg)
	}
	return m, cmd
}

func (m AppModel) activeSectionKeys() []HelpEntry {
	switch m.activeTab {
	case TabBoard:
		return BoardKeys
	case TabRepos:
		switch m.repos.activePane {
		case reposPaneBranches:
			return ReposBranchesKeys
		case reposPaneTags:
			return ReposTagsKeys
		case reposPaneMRs:
			return ReposSubMRsKeys
		case reposPanePipelines:
			return ReposPipelinesKeys
		default:
			return ReposMainKeys
		}
	case TabMRs:
		if m.mrs.activePane == mrsPanePipelines {
			return MRsPipelinesKeys
		}
		return MRsMainKeys
	case TabKube:
		switch m.kube.activePane {
		case kubePanePods:
			return KubePodsKeys
		case kubePaneServices:
			return KubeServicesKeys
		case kubePaneDeployments:
			return KubeDeploymentsKeys
		default:
			return KubeConfigsKeys
		}
	case TabChats:
		switch m.chats.activePane {
		case ChatsPaneBottom:
			return ChatsNotesKeys
		}
		return ChatsTopKeys
	}
	return nil
}

// activeSectionName returns a descriptive name for the help modal title,
// including the active sub-pane where applicable.
func (m AppModel) activeSectionName() string {
	switch m.activeTab {
	case TabBoard:
		return "Sprint Board"
	case TabRepos:
		switch m.repos.activePane {
		case reposPaneBranches:
			return "Repositories \u2014 Branches"
		case reposPaneTags:
			return "Repositories \u2014 Tags"
		case reposPaneMRs:
			return "Repositories \u2014 Merge Requests"
		case reposPanePipelines:
			return "Repositories \u2014 Pipelines"
		}
		return "Repositories"
	case TabMRs:
		if m.mrs.activePane == mrsPanePipelines {
			return "Merge Requests \u2014 Pipelines"
		}
		return "Merge Requests"
	case TabKube:
		switch m.kube.activePane {
		case kubePanePods:
			return "Kubernetes \u2014 Pods"
		case kubePaneServices:
			return "Kubernetes \u2014 Services"
		case kubePaneDeployments:
			return "Kubernetes \u2014 Deployments"
		}
		return "Kubernetes \u2014 Configs"
	case TabChats:
		if m.chats.activePane == ChatsPaneBottom {
			return "Chats — Notes & Templates"
		}
		return "Chats"
	}
	return tabLabels[m.activeTab]
}

// View renders the full TUI.
func (m AppModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	tabBar := m.renderTabBar()
	// Reserve rows: tab bar, separator, status bar, optional error banner
	extraRows := 3
	if m.errorMsg != "" {
		extraRows = 4
	}
	contentH := m.height - extraRows
	if contentH < 1 {
		contentH = 1
	}
	content := m.renderActiveSection(contentH)
	statusBar := m.renderStatusBar()

	separator := lipgloss.NewStyle().
		Foreground(colorBorder).
		Render(strings.Repeat("─", m.width))

	parts := []string{tabBar, separator, content, statusBar}

	if m.errorMsg != "" {
		errBar := lipgloss.NewStyle().
			Background(lipgloss.Color("160")).
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Width(m.width).
			Render("  ✗ " + m.errorMsg + "  (copied to clipboard · esc to dismiss)")
		parts = append(parts, errBar)
	}

	full := strings.Join(parts, "\n")

	// All modals are centred over the screen content.
	if m.activeModal != nil {
		modalView := m.activeModal.view(m.width, m.height)
		if cp, ok := m.activeModal.(CommandPaletteModal); ok {
			full = overlayHCenterAt(full, modalView, cp.AnchorY, m.width, m.height)
		} else {
			full = overlayCenter(full, modalView, m.width, m.height)
		}
	}

	return full
}

func (m AppModel) renderTabBar() string {
	tabs := make([]string, len(tabLabels)+1)
	for i, label := range tabLabels {
		if Tab(i) == m.activeTab {
			tabs[i] = tabActiveStyle.Render(label)
		} else {
			tabs[i] = tabInactiveStyle.Render(label)
		}
	}

	var zeroTab string
	switch {
	case m.refreshPending > 0:
		frame := spinFrames[m.spinFrame%len(spinFrames)]
		zeroTab = tabInactiveStyle.Foreground(colorYellow).Render(frame + " [0] refresh")
	case !m.refreshDoneAt.IsZero() && time.Since(m.refreshDoneAt) < 2*time.Second && !m.refreshFailed:
		zeroTab = tabInactiveStyle.Foreground(colorGreen).Render("[0] refresh ✓")
	case !m.refreshDoneAt.IsZero() && time.Since(m.refreshDoneAt) < 2*time.Second && m.refreshFailed:
		zeroTab = tabInactiveStyle.Foreground(colorRed).Render("[0] refresh ✗")
	default:
		zeroTab = tabInactiveStyle.Render("[0] refresh")
	}
	tabs[len(tabLabels)] = zeroTab

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m AppModel) renderActiveSection(height int) string {
	switch m.activeTab {
	case TabBoard:
		return m.board.view(m.width, height)
	case TabRepos:
		return m.repos.view(m.width, height)
	case TabMRs:
		return m.mrs.view(m.width, height)
	case TabKube:
		return m.kube.view(m.width, height)
	case TabChats:
		return m.chats.view(m.width, height)
	}
	return ""
}

func (m AppModel) renderStatusBar() string {
	hints := []string{
		"↑/↓: navigate",
		"→: commands",
		"tab: switch pane",
		"1-5: switch tab",
		"0: refresh",
		"?: help",
		"q: quit",
	}
	if m.debug {
		hints = append(hints, "DEBUG")
	}
	bar := strings.Join(hints, "  │  ")
	return statusBarStyle.Width(m.width).Render(bar)
}
