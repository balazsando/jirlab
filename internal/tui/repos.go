package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

// reposLoadedMsg is delivered when the repo scanner finishes.
type reposLoadedMsg struct{ repos []service.Repo }

// reposWithProjectsMsg is delivered after GitLab project IDs are resolved.
type reposWithProjectsMsg struct{ repos []service.Repo }

// repoActionDoneMsg is delivered after a git command completes on a repo.
type repoActionDoneMsg struct {
	message string
	rescan  bool
}

// repoBranchesLoadedMsg carries branches for the current repo.
type repoBranchesLoadedMsg struct {
	repoPath string
	branches []service.Branch
	err      error
}

// repoTagsLoadedMsg carries tags for the current repo.
type repoTagsLoadedMsg struct {
	repoPath string
	tags     []service.Tag
	err      error
}

// repoSubMRsLoadedMsg carries MRs for the current repo subtab.
type repoSubMRsLoadedMsg struct {
	repoPath string
	mrs      []service.MergeRequest
	err      error
}

// repoPipelinesLoadedMsg carries active pipelines for the current repo.
type repoPipelinesLoadedMsg struct {
	repoPath  string
	pipelines []service.Pipeline
	err       error
}

// repoLatestTagMsg carries the newest GitLab tag name and dates for a single repo,
// used to indicate when a repo's version is behind the latest release.
type repoLatestTagMsg struct {
	repoPath           string
	latestTag          string    // empty string means no tag found or fetch failed
	latestTagDate      time.Time // creation date of the latest tag
	currentVersionDate time.Time // creation date of the tag matching the repo's Version
}

// repoCIVariablesLoadedMsg is delivered after fetching configurable CI variables
// from .gitlab-ci.yml for the selected repository.
type repoCIVariablesLoadedMsg struct {
	repo service.Repo
	ref  string
	vars []service.PipelineVariable
	err  error
}

// ---------------------------------------------------------------------------
// Repo-level fetch commands
// ---------------------------------------------------------------------------

// ScanReposCmd returns a tea.Cmd that scans for repos.
// If REPOS_DIR is set it is used as the root; otherwise the user home directory is used.
func ScanReposCmd() tea.Cmd {
	return func() tea.Msg {
		root := os.Getenv("REPOS_DIR")
		if root == "" {
			var err error
			root, err = os.UserHomeDir()
			if err != nil || root == "" {
				root = os.Getenv("HOME")
			}
			if root == "" {
				root = os.Getenv("USERPROFILE") // Windows fallback
			}
			if root == "" {
				root = "/"
			}
		}
		repos, scanErr := service.ScanRepos(root)
		if scanErr != nil {
			return reposLoadedMsg{repos: repos}
		}
		return reposLoadedMsg{repos: repos}
	}
}

// resolveRepoProjectsCmd resolves GitLab project IDs for discovered repos.
func resolveRepoProjectsCmd(repos []service.Repo, client service.GitLabClient, apiBaseURL string) tea.Cmd {
	return func() tea.Msg {
		resolved := service.ResolveGitLabProjectIDs(repos, client, apiBaseURL)
		return reposWithProjectsMsg{repos: resolved}
	}
}

func fetchRepoBranchesCmd(gitlab integration.GitLabService, repo service.Repo) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil || repo.GitLabProjectID == 0 {
			return repoBranchesLoadedMsg{repoPath: repo.Path, err: fmt.Errorf("GitLab not configured for %s", repo.Name)}
		}
		defaultBranch := repoDefaultBranch(repo)
		branches, err := gitlab.GetProjectBranches(repo.GitLabProjectID, defaultBranch)
		return repoBranchesLoadedMsg{repoPath: repo.Path, branches: branches, err: err}
	}
}

func fetchRepoTagsCmd(gitlab integration.GitLabService, repo service.Repo) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil || repo.GitLabProjectID == 0 {
			return repoTagsLoadedMsg{repoPath: repo.Path, err: fmt.Errorf("GitLab not configured for %s", repo.Name)}
		}
		tags, err := gitlab.GetProjectTags(repo.GitLabProjectID)
		return repoTagsLoadedMsg{repoPath: repo.Path, tags: tags, err: err}
	}
}

func fetchRepoSubMRsCmd(gitlab integration.GitLabService, repo service.Repo) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil || repo.GitLabProjectID == 0 {
			return repoSubMRsLoadedMsg{repoPath: repo.Path, err: fmt.Errorf("GitLab not configured for %s", repo.Name)}
		}
		mrs, err := gitlab.GetProjectMRs(repo.GitLabProjectID)
		return repoSubMRsLoadedMsg{repoPath: repo.Path, mrs: mrs, err: err}
	}
}

func fetchRepoPipelinesCmd(gitlab integration.GitLabService, repo service.Repo) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil || repo.GitLabProjectID == 0 {
			return repoPipelinesLoadedMsg{repoPath: repo.Path, err: fmt.Errorf("GitLab not configured for %s", repo.Name)}
		}
		pipelines, err := gitlab.GetProjectPipelines(repo.GitLabProjectID, repo.Name, []string{"running", "pending", "failed"})
		return repoPipelinesLoadedMsg{repoPath: repo.Path, pipelines: pipelines, err: err}
	}
}

// fetchRepoLatestTagCmd returns a command that fetches the newest tag for a repo
// so the version column can indicate whether the project is behind the latest release.
// Errors are silently swallowed (best-effort, supplementary information).
func fetchRepoLatestTagCmd(gitlab integration.GitLabService, repo service.Repo) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil || repo.GitLabProjectID == 0 {
			return repoLatestTagMsg{repoPath: repo.Path}
		}
		tags, err := gitlab.GetProjectTags(repo.GitLabProjectID)
		if err != nil || len(tags) == 0 {
			return repoLatestTagMsg{repoPath: repo.Path}
		}
		latest := tags[0]
		var currentVersionDate time.Time
		for _, t := range tags {
			if t.Name == repo.Version {
				currentVersionDate = t.CreatedAt
				break
			}
		}
		return repoLatestTagMsg{
			repoPath:           repo.Path,
			latestTag:          latest.Name,
			latestTagDate:      latest.CreatedAt,
			currentVersionDate: currentVersionDate,
		}
	}
}

// ---------------------------------------------------------------------------
// Subtab pane enum
// ---------------------------------------------------------------------------

type reposPane int

const (
	reposPaneMain reposPane = iota
	reposPaneBranches
	reposPaneTags
	reposPaneMRs
	reposPanePipelines
)

// ---------------------------------------------------------------------------
// ReposSection
// ---------------------------------------------------------------------------

// ReposSection is the Repositories tab.
type ReposSection struct {
	repos             []service.Repo
	filtered          []service.Repo
	cursor            int
	loading           bool
	statusMsg         string
	savedRepoName     string // repo name to restore cursor to after a rescan
	mrStatuses        map[string]mrInfo
	projectMRStatuses map[int]mrInfo  // project ID → MR info
	sprintIssueKeys   map[string]bool // issue keys currently in the sprint
	git               integration.GitCmdService
	shell             integration.ShellService
	gitlab            integration.GitLabService
	termHeight        int // terminal height, set on resize

	// Subtab state
	activePane  reposPane
	lastSubPane reposPane
	subRepoPath string // which repo path the loaded subtab data belongs to

	// Branches subtab
	branches      []service.Branch
	branchCursor  int
	branchLoading bool
	branchErr     string

	// Tags subtab
	tags       []service.Tag
	tagCursor  int
	tagLoading bool
	tagErr     string

	// MRs subtab
	subMRs       []service.MergeRequest
	subMRCursor  int
	subMRLoading bool
	subMRErr     string

	// Pipelines subtab
	repoPipelines   []service.Pipeline
	pipelineCursor  int
	pipelineLoading bool
	pipelineErr     string
}

func newReposSection(git integration.GitCmdService, shell integration.ShellService, gitlab integration.GitLabService) ReposSection {
	return ReposSection{
		loading:           true,
		mrStatuses:        make(map[string]mrInfo),
		projectMRStatuses: make(map[int]mrInfo),
		git:               git,
		shell:             shell,
		gitlab:            gitlab,
		activePane:        reposPaneMain,
		lastSubPane:       reposPaneBranches,
	}
}

func (s ReposSection) update(msg tea.Msg) (ReposSection, tea.Cmd) {
	switch msg := msg.(type) {
	case reposLoadedMsg:
		s.repos = msg.repos
		s.filtered = msg.repos
		s.loading = false
		s.statusMsg = ""
		// Restore cursor to the previously-selected repo (by name) if possible,
		// otherwise default to 0.
		if s.savedRepoName != "" {
			s.cursor = 0
			for i, r := range s.filtered {
				if r.Name == s.savedRepoName {
					s.cursor = i
					break
				}
			}
			s.savedRepoName = ""
		} else {
			s.cursor = 0
		}

	case reposWithProjectsMsg:
		s.repos = msg.repos
		s.filtered = msg.repos
		return s.fetchCurrentSubtabCmd()

	case repoActionDoneMsg:
		s.statusMsg = msg.message
		if msg.rescan {
			if repo := s.currentRepo(); repo != nil {
				s.savedRepoName = repo.Name
			}
			return s, ScanReposCmd()
		}

	case clipboardCopiedMsg:
		s.statusMsg = "Copied: " + truncStr(msg.text, 30)

	case repoBranchesLoadedMsg:
		s.branchLoading = false
		if msg.err != nil {
			s.branchErr = msg.err.Error()
		} else {
			s.branchErr = ""
			s.branches = msg.branches
			s.branchCursor = 0
		}
		s.subRepoPath = msg.repoPath

	case repoTagsLoadedMsg:
		s.tagLoading = false
		if msg.err != nil {
			s.tagErr = msg.err.Error()
		} else {
			s.tagErr = ""
			s.tags = msg.tags
			s.tagCursor = 0
		}
		s.subRepoPath = msg.repoPath

	case repoSubMRsLoadedMsg:
		s.subMRLoading = false
		if msg.err != nil {
			s.subMRErr = msg.err.Error()
		} else {
			s.subMRErr = ""
			s.subMRs = msg.mrs
			sort.SliceStable(s.subMRs, func(i, j int) bool {
				return s.subMRs[i].IID > s.subMRs[j].IID
			})
			s.subMRCursor = 0
		}
		s.subRepoPath = msg.repoPath

	case repoPipelinesLoadedMsg:
		s.pipelineLoading = false
		if msg.err != nil {
			s.pipelineErr = msg.err.Error()
		} else {
			s.pipelineErr = ""
			cutoff := time.Now().Add(-24 * time.Hour)
			var relevant []service.Pipeline
			for _, p := range msg.pipelines {
				switch p.Status {
				case "running", "pending":
					relevant = append(relevant, p)
				case "failed":
					// Use UpdatedAt when available, fall back to CreatedAt
					ts := p.UpdatedAt
					if ts.IsZero() {
						ts = p.CreatedAt
					}
					if ts.After(cutoff) {
						relevant = append(relevant, p)
					}
				}
			}
			s.repoPipelines = relevant
			s.pipelineCursor = 0
		}
		s.subRepoPath = msg.repoPath

	case repoLatestTagMsg:
		for i := range s.repos {
			if s.repos[i].Path == msg.repoPath {
				s.repos[i].LatestTagVersion = msg.latestTag
				s.repos[i].LatestTagDate = msg.latestTagDate
				s.repos[i].CurrentVersionDate = msg.currentVersionDate
				break
			}
		}
		// Rebuild filtered to ensure the update is reflected in any copy.
		s.filtered = s.repos

	case repoCIVariablesLoadedMsg:
		if msg.err != nil {
			s.statusMsg = "CI vars: " + msg.err.Error()
			return s, nil
		}
		r := msg.repo
		ref := msg.ref
		glab := s.gitlab
		return s, func() tea.Msg {
			return OpenModalMsg{M: NewPipelineTriggerModal(r.Name, ref, msg.vars,
				func(values map[string]string) tea.Cmd {
					return triggerPipelineCmd(glab, r, ref, values)
				},
			)}
		}

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (s ReposSection) handleKey(msg tea.KeyMsg) (ReposSection, tea.Cmd) {
	switch msg.String() {
	case "tab":
		if s.activePane == reposPaneMain {
			s.activePane = s.lastSubPane
		} else {
			s.activePane = reposPaneMain
		}
		return s, nil

	case "j", "down":
		switch s.activePane {
		case reposPaneMain:
			if s.cursor < len(s.filtered)-1 {
				s.cursor++
			}
			return s.fetchCurrentSubtabCmd()
		case reposPaneBranches:
			if s.branchCursor < len(s.branches)-1 {
				s.branchCursor++
			}
		case reposPaneTags:
			if s.tagCursor < len(s.tags)-1 {
				s.tagCursor++
			}
		case reposPaneMRs:
			if s.subMRCursor < len(s.subMRs)-1 {
				s.subMRCursor++
			}
		case reposPanePipelines:
			if s.pipelineCursor < len(s.repoPipelines)-1 {
				s.pipelineCursor++
			}
		}
		return s, nil

	case "k", "up":
		switch s.activePane {
		case reposPaneMain:
			if s.cursor > 0 {
				s.cursor--
			}
			return s.fetchCurrentSubtabCmd()
		case reposPaneBranches:
			if s.branchCursor > 0 {
				s.branchCursor--
			}
		case reposPaneTags:
			if s.tagCursor > 0 {
				s.tagCursor--
			}
		case reposPaneMRs:
			if s.subMRCursor > 0 {
				s.subMRCursor--
			}
		case reposPanePipelines:
			if s.pipelineCursor > 0 {
				s.pipelineCursor--
			}
		}
		return s, nil

	case "enter":
		switch s.activePane {
		case reposPaneMain:
			if repo := s.currentRepo(); repo != nil {
				return s, navigateCmd(s.git, repo.Path)
			}
		case reposPaneBranches:
			if repo := s.currentRepo(); repo != nil && s.branchCursor < len(s.branches) {
				br := s.branches[s.branchCursor]
				return s, checkoutRepoBranchCmd(s.git, *repo, br.Name)
			}
		case reposPaneTags:
			if repo := s.currentRepo(); repo != nil && s.tagCursor < len(s.tags) {
				tag := s.tags[s.tagCursor]
				return s, checkoutRepoTagCmd(s.git, *repo, tag.Name)
			}
		case reposPaneMRs:
			if repo := s.currentRepo(); repo != nil && s.subMRCursor < len(s.subMRs) {
				mr := s.subMRs[s.subMRCursor]
				return s, checkoutRepoMRBranchCmd(s.git, *repo, mr)
			}
		case reposPanePipelines:
			if s.pipelineCursor < len(s.repoPipelines) {
				p := s.repoPipelines[s.pipelineCursor]
				if p.WebURL != "" {
					return s, openBrowserCmd(s.shell, p.WebURL)
				}
			}
		}
		return s, nil

	case "n":
		if s.activePane == reposPaneMain {
			if repo := s.currentRepo(); repo != nil {
				return s, navigateCmd(s.git, repo.Path)
			}
		}
		return s, nil

	// Subtab switch keys — switch visible subtab and re-fetch for current repo.
	case "r":
		s.activePane = reposPaneBranches
		s.lastSubPane = reposPaneBranches
		return s.fetchCurrentSubtabCmd()
	case "t":
		s.activePane = reposPaneTags
		s.lastSubPane = reposPaneTags
		return s.fetchCurrentSubtabCmd()
	case "m":
		s.activePane = reposPaneMRs
		s.lastSubPane = reposPaneMRs
		return s.fetchCurrentSubtabCmd()
	case "p":
		s.activePane = reposPanePipelines
		s.lastSubPane = reposPanePipelines
		return s.fetchCurrentSubtabCmd()

	// Main-pane-only keys
	case "b":
		if s.activePane == reposPaneMain {
			if repo := s.currentRepo(); repo != nil {
				r := *repo
				git := s.git
				return s, func() tea.Msg {
					return OpenModalMsg{M: NewInputModal(
						fmt.Sprintf("New branch in %s", r.Name),
						"feature/my-branch",
						func(branchName string) tea.Cmd {
							return createLocalBranchCmd(git, r, branchName)
						},
					)}
				}
			}
		}
	case "o":
		if s.activePane == reposPaneMain {
			if repo := s.currentRepo(); repo != nil {
				path := repo.Path
				return s, func() tea.Msg {
					return OpenModalMsg{M: EditorSelectModal{RepoPath: path, Shell: s.shell}}
				}
			}
		}
	case "w":
		switch s.activePane {
		case reposPaneMain:
			if repo := s.currentRepo(); repo != nil && repo.WebURL != "" {
				return s, openBrowserCmd(s.shell, repo.WebURL)
			}
		case reposPaneMRs:
			if s.subMRCursor < len(s.subMRs) {
				if mr := s.subMRs[s.subMRCursor]; mr.WebURL != "" {
					return s, openBrowserCmd(s.shell, mr.WebURL)
				}
			}
		case reposPanePipelines:
			if s.pipelineCursor < len(s.repoPipelines) {
				if p := s.repoPipelines[s.pipelineCursor]; p.WebURL != "" {
					return s, openBrowserCmd(s.shell, p.WebURL)
				}
			}
		}
	case "d":
		if s.activePane == reposPaneMain {
			if repo := s.currentRepo(); repo != nil {
				return s, checkoutDevelopCmd(s.git, *repo)
			}
		}
	case "v":
		if s.activePane == reposPaneMain {
			if repo := s.currentRepo(); repo != nil {
				if repo.Version == "" {
					s.statusMsg = "No version found (.mvn/maven.config missing -Drevision)"
					return s, nil
				}
				return s, copyToClipboard(s.shell, repo.Version)
			}
		}
	case "c":
		if s.activePane == reposPaneMain {
			if repo := s.currentRepo(); repo != nil {
				if repo.GitLabProjectID == 0 {
					s.statusMsg = "GitLab project not resolved for this repo"
					return s, nil
				}
				if repo.CurrentBranch == "" {
					s.statusMsg = "No branch checked out"
					return s, nil
				}
				targetBranch := repoDefaultBranch(*repo)
				if repo.CurrentBranch == targetBranch {
					s.statusMsg = fmt.Sprintf("Already on %s — nothing to merge", targetBranch)
					return s, nil
				}
				r, glab := *repo, s.gitlab
				shell := s.shell
				defaultTitle := fmt.Sprintf("Merge Request for %s", r.CurrentBranch)
				return s, func() tea.Msg {
					return OpenModalMsg{M: NewInputModalWithValue(
						fmt.Sprintf("Create MR: %s → %s", r.CurrentBranch, targetBranch),
						defaultTitle,
						func(title string) tea.Cmd {
							return createMRCmd(glab, shell, r, targetBranch, title)
						},
					)}
				}
			}
		}
	case "i":
		if repo := s.currentRepo(); repo != nil {
			if repo.GitLabProjectID == 0 {
				s.statusMsg = "GitLab project not resolved for this repo"
				return s, nil
			}
			return s, openRefSelectCmd(s.gitlab, *repo)
		}
	case "g":
		if s.activePane == reposPaneMain {
			if repo := s.currentRepo(); repo != nil {
				if repo.CurrentBranch == "" {
					s.statusMsg = "No branch checked out"
					return s, nil
				}
				return s, stageAllCommitPushCmd(s.git, *repo)
			}
		}
	case "right":
		if repo := s.currentRepo(); repo != nil {
			commands := s.buildCommandPalette(*repo)
			if len(commands) > 0 {
				// Estimate screen row for the active cursor to anchor the palette.
				var anchorY int
				repoListH := s.termHeight / 2
				if repoListH < 4 {
					repoListH = 4
				}
				maxRepoRows := repoListH - 3
				if maxRepoRows < 1 {
					maxRepoRows = 1
				}
				switch s.activePane {
				case reposPaneMain:
					start, _ := scrollWindow(s.cursor, len(s.filtered), maxRepoRows)
					anchorY = 4 + (s.cursor - start)
				case reposPaneBranches:
					start, _ := scrollWindow(s.branchCursor, len(s.branches), maxRepoRows)
					anchorY = repoListH + 6 + (s.branchCursor - start)
				case reposPaneTags:
					start, _ := scrollWindow(s.tagCursor, len(s.tags), maxRepoRows)
					anchorY = repoListH + 6 + (s.tagCursor - start)
				case reposPaneMRs:
					start, _ := scrollWindow(s.subMRCursor, len(s.subMRs), maxRepoRows)
					anchorY = repoListH + 6 + (s.subMRCursor - start)
				case reposPanePipelines:
					start, _ := scrollWindow(s.pipelineCursor, len(s.repoPipelines), maxRepoRows)
					anchorY = repoListH + 6 + (s.pipelineCursor - start)
				}
				return s, func() tea.Msg {
					return OpenModalMsg{M: CommandPaletteModal{commands: commands, AnchorY: anchorY}}
				}
			}
		}
	}
	return s, nil
}

func (s *ReposSection) currentRepo() *service.Repo {
	if len(s.filtered) == 0 || s.cursor >= len(s.filtered) {
		return nil
	}
	r := s.filtered[s.cursor]
	return &r
}

// fetchCurrentSubtabCmd sets the appropriate loading flag and returns a fetch
// command for the active subtab of the currently selected repository.
func (s ReposSection) fetchCurrentSubtabCmd() (ReposSection, tea.Cmd) {
	repo := s.currentRepo()
	if repo == nil {
		return s, nil
	}
	switch s.lastSubPane {
	case reposPaneBranches:
		s.branchLoading = true
		s.branchErr = ""
		return s, fetchRepoBranchesCmd(s.gitlab, *repo)
	case reposPaneTags:
		s.tagLoading = true
		s.tagErr = ""
		return s, fetchRepoTagsCmd(s.gitlab, *repo)
	case reposPaneMRs:
		s.subMRLoading = true
		s.subMRErr = ""
		return s, fetchRepoSubMRsCmd(s.gitlab, *repo)
	case reposPanePipelines:
		s.pipelineLoading = true
		s.pipelineErr = ""
		return s, fetchRepoPipelinesCmd(s.gitlab, *repo)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (s ReposSection) view(width, height int) string {
	repoListH := height / 2
	if repoListH < 4 {
		repoListH = 4
	}
	// overhead: sep(1) + paneBar(1) + paneSep(1) + statusMsg(1) = 4 extra rows
	subtabH := height - repoListH - 5
	if subtabH < 3 {
		subtabH = 3
	}

	upper := s.viewReposList(width, repoListH)
	sep := sectionSepLine(width)
	paneBar := s.viewSubPaneBar(width)
	paneSep := sectionSepLine(width)
	lower := s.viewSubPane(width, subtabH)

	parts := []string{upper, sep, paneBar, paneSep, lower}
	if s.statusMsg != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render("  "+s.statusMsg))
	}
	return strings.Join(parts, "\n")
}

func (s ReposSection) viewSubPaneBar(width int) string {
	type tabDef struct {
		pane  reposPane
		label string
	}
	tabs := []tabDef{
		{reposPaneBranches, "[r] Branches"},
		{reposPaneTags, "[t] Tags"},
		{reposPaneMRs, "[m] MRs"},
		{reposPanePipelines, "[p] Pipelines"},
	}
	topWidths := [4]int{
		lipgloss.Width(tabInactiveStyle.Render(tabLabels[0])),
		lipgloss.Width(tabInactiveStyle.Render(tabLabels[1])),
		lipgloss.Width(tabInactiveStyle.Render(tabLabels[2])),
		lipgloss.Width(tabInactiveStyle.Render(tabLabels[3])),
	}
	var parts []string
	for i, t := range tabs {
		w := topWidths[i]
		if t.pane == s.lastSubPane {
			if s.activePane == reposPaneMain {
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

func (s ReposSection) viewReposList(width, height int) string {
	if s.loading && len(s.filtered) == 0 {
		return headerStyle.Render("  Scanning repositories…")
	}
	if len(s.filtered) == 0 {
		return headerStyle.Render("  No repositories found (no .gitlab-ci.yml in HOME)")
	}

	statsW := 14
	versionW := 18
	nameW := width / 4
	if nameW < 16 {
		nameW = 16
	}
	if nameW > 30 {
		nameW = 30
	}
	branchW := width - nameW - statsW - versionW - 10
	if branchW < 12 {
		branchW = 12
	}

	colH := func(str string, w int) string {
		return columnHeaderStyle.Width(w).Render(truncStr(str, w))
	}
	header := colH("REPOSITORY", nameW) + " " + colH("BRANCH", branchW) + " " + colH("VERSION", versionW) + " " + colH("CHANGES", statsW)

	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.cursor, len(s.filtered), maxRows)

	var rows []string
	rows = append(rows, header, tableHeaderSepLine(width))
	for i := start; i < end; i++ {
		r := s.filtered[i]
		branch := r.CurrentBranch
		if branch == "" {
			branch = "(no branch)"
		}
		stats := repoGitStatsCell(r.GitStats)
		verCell := repoVersionCell(r, versionW)
		line := fmt.Sprintf("%-*s %-*s %s %s",
			nameW, truncStr(r.Name, nameW),
			branchW, truncStr(branch, branchW),
			verCell,
			stats,
		)
		if i == s.cursor && s.activePane == reposPaneMain {
			rows = append(rows, selectedRowStyle.Foreground(s.repoColor(r)).Width(width).Render(line))
		} else if i == s.cursor {
			rows = append(rows, selectedRowDimStyle.Foreground(s.repoColor(r)).Width(width).Render(line))
		} else {
			rows = append(rows, s.repoRowStyle(r).Width(width).Render(line))
		}
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, RepoColorLegendBar(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Render(
		"  enter: navigate  d: default  b: new branch  g: commit & push  i: trigger pipeline  →: all commands",
	))
	return strings.Join(rows, "\n")
}

func (s ReposSection) viewSubPane(width, height int) string {
	repo := s.currentRepo()
	switch s.lastSubPane {
	case reposPaneBranches:
		return s.viewBranchesPane(width, height, repo)
	case reposPaneTags:
		return s.viewTagsPane(width, height, repo)
	case reposPaneMRs:
		return s.viewSubMRsPane(width, height, repo)
	case reposPanePipelines:
		repoName := ""
		if repo != nil {
			repoName = repo.Name
		}
		return s.viewPipelinesPane(width, height, repoName)
	}
	return lipgloss.NewStyle().Foreground(colorMuted).Render("  Press r/t/m/p to load subtab data")
}

func (s ReposSection) viewBranchesPane(width, height int, repo *service.Repo) string {
	if p := subPanePlaceholder(s.branchLoading, len(s.branches), "Loading branches…", s.branchErr, "No branches found"); p != "" {
		return p
	}

	activeBranch := ""
	if repo != nil {
		activeBranch = repo.CurrentBranch
	}
	protW := 3
	nameW := width - protW - 6
	if nameW < 20 {
		nameW = 20
	}

	colH := func(str string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(str, w)) }
	header := colH("BRANCH", nameW) + " " + colH("", protW)

	var rows []string
	rows = append(rows, header, tableHeaderSepLine(width))

	maxRows := height - 4
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.branchCursor, len(s.branches), maxRows)

	for i := start; i < end; i++ {
		br := s.branches[i]
		prot := ""
		if br.Protected {
			prot = lipgloss.NewStyle().Foreground(colorMuted).Render("🔒")
		}
		line := fmt.Sprintf("%-*s %s", nameW, truncStr(br.Name, nameW), prot)
		brCol := colorFg
		if activeBranch != "" && br.Name == activeBranch {
			brCol = colorGreen
		} else if br.Protected {
			brCol = colorMuted
		}
		if i == s.branchCursor && s.activePane == reposPaneBranches {
			rows = append(rows, selectedRowStyle.Foreground(brCol).Width(width).Render(line))
		} else if i == s.branchCursor {
			rows = append(rows, selectedRowDimStyle.Foreground(brCol).Width(width).Render(line))
		} else {
			rows = append(rows, normalRowStyle.Foreground(brCol).Width(width).Render(line))
		}
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, BranchColorLegendBar(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Render(
		"  enter: checkout  w: open in browser  →: more commands",
	))
	return strings.Join(rows, "\n")
}

func (s ReposSection) viewTagsPane(width, height int, repo *service.Repo) string {
	if p := subPanePlaceholder(s.tagLoading, len(s.tags), "Loading tags…", s.tagErr, "No tags found"); p != "" {
		return p
	}

	dateW := 12
	nameW := width - dateW - 6
	if nameW < 20 {
		nameW = 20
	}

	colH := func(str string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(str, w)) }
	header := colH("TAG", nameW) + " " + colH("DATE", dateW)

	var rows []string
	rows = append(rows, header, tableHeaderSepLine(width))

	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.tagCursor, len(s.tags), maxRows)

	for i := start; i < end; i++ {
		tag := s.tags[i]
		dateStr := "-"
		if !tag.CreatedAt.IsZero() {
			dateStr = tag.CreatedAt.Format("2006-01-02")
		}
		line := fmt.Sprintf("%-*s %-*s", nameW, truncStr(tag.Name, nameW), dateW, dateStr)
		if i == s.tagCursor && s.activePane == reposPaneTags {
			rows = append(rows, selectedRowStyle.Foreground(colorFg).Width(width).Render(line))
		} else if i == s.tagCursor {
			rows = append(rows, selectedRowDimStyle.Foreground(colorFg).Width(width).Render(line))
		} else {
			rows = append(rows, normalRowStyle.Foreground(colorFg).Width(width).Render(line))
		}
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Render(
		"  enter: checkout tag  →: more commands",
	))
	return strings.Join(rows, "\n")
}

func (s ReposSection) viewSubMRsPane(width, height int, repo *service.Repo) string {
	if p := subPanePlaceholder(s.subMRLoading, len(s.subMRs), "Loading merge requests…", s.subMRErr, "No open merge requests"); p != "" {
		return p
	}

	activeBranch := ""
	if repo != nil {
		activeBranch = repo.CurrentBranch
	}
	authorW := 16
	statusW := 10
	titleW := width - authorW - statusW - 6
	if titleW < 15 {
		titleW = 15
	}

	colH := func(str string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(str, w)) }
	header := colH("TITLE", titleW) + " " + colH("AUTHOR", authorW) + " " + colH("STATUS", statusW)

	var rows []string
	rows = append(rows, header, tableHeaderSepLine(width))

	maxRows := height - 4
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.subMRCursor, len(s.subMRs), maxRows)

	for i := start; i < end; i++ {
		mr := s.subMRs[i]
		line := fmt.Sprintf("%-*s %-*s %-*s",
			titleW, truncStr(mr.Title, titleW),
			authorW, truncStr(mr.Author, authorW),
			statusW, truncStr(mr.Status, statusW),
		)
		mrCol := MRColor(mr)
		if activeBranch != "" && mr.SourceBranch == activeBranch {
			mrCol = colorGreen
		}
		if i == s.subMRCursor && s.activePane == reposPaneMRs {
			rows = append(rows, selectedRowStyle.Foreground(mrCol).Width(width).Render(line))
		} else if i == s.subMRCursor {
			rows = append(rows, selectedRowDimStyle.Foreground(mrCol).Width(width).Render(line))
		} else {
			rows = append(rows, normalRowStyle.Foreground(mrCol).Width(width).Render(line))
		}
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, MRColorLegendBar(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Render(
		"  enter: checkout branch  w: open MR in browser  →: more commands",
	))
	return strings.Join(rows, "\n")
}

func (s ReposSection) viewPipelinesPane(width, height int, repoName string) string {
	if p := subPanePlaceholder(s.pipelineLoading, len(s.repoPipelines), "Loading pipelines…", s.pipelineErr, "No active pipelines (running/pending)"); p != "" {
		return p
	}

	statusW := 10
	ageW := 8
	refW := width - statusW - ageW - 6
	if refW < 20 {
		refW = 20
	}

	colH := func(str string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(str, w)) }
	header := colH("REF", refW) + " " + colH("STATUS", statusW) + " " + colH("AGE", ageW)

	var rows []string
	rows = append(rows, header, tableHeaderSepLine(width))

	maxRows := height - 4
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.pipelineCursor, len(s.repoPipelines), maxRows)
	now := time.Now()

	for i := start; i < end; i++ {
		p := s.repoPipelines[i]
		age := "-"
		if !p.CreatedAt.IsZero() {
			age = kubeAge(p.CreatedAt.Format(time.RFC3339), now)
		}
		line := fmt.Sprintf("%-*s %-*s %-*s",
			refW, truncStr(p.Ref, refW),
			statusW, truncStr(p.Status, statusW),
			ageW, age,
		)
		col := pipelineStatusColor(p.Status)
		if i == s.pipelineCursor && s.activePane == reposPanePipelines {
			rows = append(rows, selectedRowStyle.Foreground(col).Width(width).Render(line))
		} else if i == s.pipelineCursor {
			rows = append(rows, selectedRowDimStyle.Foreground(col).Width(width).Render(line))
		} else {
			rows = append(rows, normalRowStyle.Foreground(col).Width(width).Render(line))
		}
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, PipelineStatusLegendBar(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Render(
		"  enter/w: open in browser  i: trigger new pipeline  →: more commands",
	))
	return strings.Join(rows, "\n")
}

// pipelineStatusColor maps pipeline status to a display colour.
func pipelineStatusColor(status string) lipgloss.Color {
	switch strings.ToLower(status) {
	case "running":
		return colorGreen
	case "pending":
		return colorYellow
	case "failed":
		return colorRed
	case "canceled", "skipped":
		return colorMuted
	default:
		return colorFg
	}
}

// ---------------------------------------------------------------------------
// Colour helpers
// ---------------------------------------------------------------------------

func (s ReposSection) repoColor(repo service.Repo) lipgloss.Color {
	key := extractIssueKey(repo.CurrentBranch)
	if key != "" {
		info, hasMR := s.mrStatuses[key]
		if hasMR {
			if !info.isMine && !info.approved {
				return colorRed
			}
			if info.isMine && info.approved {
				return colorGreen
			}
			if info.isMine {
				return colorYellow
			}
			return colorOrange
		}
		return colorLightBlue
	}
	if repo.GitLabProjectID != 0 {
		if info, ok := s.projectMRStatuses[repo.GitLabProjectID]; ok {
			if !info.isMine && !info.approved {
				return colorRed
			}
			if info.isMine && info.approved {
				return colorGreen
			}
			if info.isMine {
				return colorYellow
			}
			return colorOrange
		}
	}
	return colorFg
}

func (s ReposSection) repoRowStyle(r service.Repo) lipgloss.Style {
	return normalRowStyle.Foreground(s.repoColor(r))
}

func (s ReposSection) helpKeys() []HelpEntry { return ReposKeys }

// repoVersionCell builds a fixed-width version cell string for the repos list.
// Uses tag creation dates to determine if the repo is behind the latest release.
func repoVersionCell(r service.Repo, width int) string {
	if r.Version == "" {
		return lipgloss.NewStyle().Width(width).Render("-")
	}
	isBehind := false
	if !r.LatestTagDate.IsZero() {
		if r.CurrentVersionDate.IsZero() {
			// Current version tag not found in GitLab — behind if the name differs
			isBehind = r.LatestTagVersion != "" && r.LatestTagVersion != r.Version
		} else {
			isBehind = r.CurrentVersionDate.Before(r.LatestTagDate)
		}
	}
	if isBehind {
		indicator := lipgloss.NewStyle().Foreground(colorYellow).Render(" ↓")
		label := truncStr(r.Version, width-2) + indicator
		return lipgloss.NewStyle().Width(width).Render(label)
	}
	return lipgloss.NewStyle().Width(width).Render(truncStr(r.Version, width))
}

// repoGitStatsCell formats git status counts.
func repoGitStatsCell(gs service.GitStats) string {
	if gs.IsClean() {
		return lipgloss.NewStyle().Foreground(colorMuted).Render("✓")
	}
	var parts []string
	if gs.Staged > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render(fmt.Sprintf("●%d", gs.Staged)))
	}
	if gs.Unstaged > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorYellow).Render(fmt.Sprintf("+%d", gs.Unstaged)))
	}
	if gs.Untracked > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("?%d", gs.Untracked)))
	}
	return strings.Join(parts, " ")
}

// createMRCmd opens a new merge request on GitLab for the given repo and copies
// the MR browser URL to the clipboard on success.
func createMRCmd(gitlab integration.GitLabService, shell integration.ShellService, repo service.Repo, targetBranch, title string) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil || repo.GitLabProjectID == 0 {
			return errMsg{source: "repos", err: fmt.Errorf("GitLab not configured for %s", repo.Name)}
		}
		mr, err := gitlab.CreateMR(repo.GitLabProjectID, repo.CurrentBranch, targetBranch, title)
		if err != nil {
			return errMsg{source: "repos", err: fmt.Errorf("create MR: %w", err)}
		}
		if shell != nil && mr.WebURL != "" {
			_ = shell.CopyToClipboard(mr.WebURL)
		}
		return repoActionDoneMsg{message: fmt.Sprintf("MR created: %s", mr.WebURL)}
	}
}

// fetchCIVariablesCmd fetches configurable CI variables from .gitlab-ci.yml for the
// selected repo/ref so the pipeline trigger modal can display them.
func fetchCIVariablesCmd(gitlab integration.GitLabService, repo service.Repo, ref string) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil || repo.GitLabProjectID == 0 {
			return repoCIVariablesLoadedMsg{repo: repo, ref: ref, err: fmt.Errorf("GitLab not configured for %s", repo.Name)}
		}
		vars, err := gitlab.GetCIVariables(repo.GitLabProjectID, ref)
		return repoCIVariablesLoadedMsg{repo: repo, ref: ref, vars: vars, err: err}
	}
}

// openRefSelectCmd opens a RefSelectModal so the user can pick the pipeline ref
// (current branch vs base/default branch), then continues to fetch CI variables.
func openRefSelectCmd(gitlab integration.GitLabService, repo service.Repo) tea.Cmd {
	currentRef := repo.CurrentBranch
	if currentRef == "" {
		currentRef = repoDefaultBranch(repo)
	}
	baseRef := repoDefaultBranch(repo)
	return func() tea.Msg {
		return OpenModalMsg{M: RefSelectModal{
			currentBranch: currentRef,
			baseBranch:    baseRef,
			onSelect: func(ref string) tea.Cmd {
				return fetchCIVariablesCmd(gitlab, repo, ref)
			},
		}}
	}
}

// triggerPipelineCmd triggers a new pipeline run for the given repo/ref.
func triggerPipelineCmd(gitlab integration.GitLabService, repo service.Repo, ref string, variables map[string]string) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil {
			return repoActionDoneMsg{message: "GitLab not configured"}
		}
		p, err := gitlab.TriggerPipeline(repo.GitLabProjectID, ref, variables)
		if err != nil {
			return repoActionDoneMsg{message: "Pipeline trigger failed: " + err.Error()}
		}
		msg := fmt.Sprintf("Pipeline triggered: %s @ %s", repo.Name, ref)
		if p != nil && p.WebURL != "" {
			msg += " — " + p.WebURL
		}
		return repoActionDoneMsg{message: msg}
	}
}

// buildCommandPalette returns a command palette for the current pane and selected row.
func (s ReposSection) buildCommandPalette(repo service.Repo) []CommandEntry {
	shell := s.shell
	git := s.git
	gitlab := s.gitlab

	switch s.activePane {
	case reposPaneMain:
		var cmds []CommandEntry
		cmds = append(cmds, CommandEntry{Key: "enter", Desc: "navigate", Cmd: navigateCmd(git, repo.Path)})
		cmds = append(cmds, CommandEntry{Key: "d", Desc: "default branch", Cmd: checkoutDevelopCmd(git, repo)})
		if repo.WebURL != "" {
			cmds = append(cmds, CommandEntry{Key: "w", Desc: "open URL", Cmd: openBrowserCmd(shell, repo.WebURL)})
		}
		cmds = append(cmds, CommandEntry{Key: "b", Desc: "new branch", Cmd: func() tea.Msg {
			return OpenModalMsg{M: NewInputModal(
				fmt.Sprintf("New branch in %s", repo.Name),
				"feature/my-branch",
				func(branchName string) tea.Cmd { return createLocalBranchCmd(git, repo, branchName) },
			)}
		}})
		if repo.GitLabProjectID != 0 {
			targetBranch := repoDefaultBranch(repo)
			if repo.CurrentBranch != "" && repo.CurrentBranch != targetBranch {
				cmds = append(cmds, CommandEntry{Key: "c", Desc: "create MR", Cmd: func() tea.Msg {
					defaultTitle := fmt.Sprintf("Merge Request for %s", repo.CurrentBranch)
					return OpenModalMsg{M: NewInputModalWithValue(
						fmt.Sprintf("Create MR: %s → %s", repo.CurrentBranch, targetBranch),
						defaultTitle,
						func(title string) tea.Cmd { return createMRCmd(gitlab, shell, repo, targetBranch, title) },
					)}
				}})
			}
			cmds = append(cmds, CommandEntry{Key: "i", Desc: "trigger pipeline", Cmd: openRefSelectCmd(gitlab, repo)})
		}
		if repo.CurrentBranch != "" {
			cmds = append(cmds, CommandEntry{Key: "g", Desc: "commit & push", Cmd: stageAllCommitPushCmd(git, repo)})
		}
		cmds = append(cmds, CommandEntry{Key: "o", Desc: "editor", Cmd: func() tea.Msg {
			return OpenModalMsg{M: EditorSelectModal{RepoPath: repo.Path, Shell: shell}}
		}})
		if repo.Version != "" {
			cmds = append(cmds, CommandEntry{Key: "v", Desc: "copy version", Cmd: copyToClipboard(shell, repo.Version)})
		}
		return cmds

	case reposPaneBranches:
		if s.branchCursor >= len(s.branches) {
			return nil
		}
		br := s.branches[s.branchCursor]
		var cmds []CommandEntry
		cmds = append(cmds, CommandEntry{Key: "enter", Desc: "checkout " + br.Name, Cmd: checkoutRepoBranchCmd(git, repo, br.Name)})
		if br.WebURL != "" {
			cmds = append(cmds, CommandEntry{Key: "w", Desc: "open URL", Cmd: openBrowserCmd(shell, br.WebURL)})
		}
		return cmds

	case reposPaneTags:
		if s.tagCursor >= len(s.tags) {
			return nil
		}
		tag := s.tags[s.tagCursor]
		return []CommandEntry{
			{Key: "enter", Desc: "checkout " + tag.Name, Cmd: checkoutRepoTagCmd(git, repo, tag.Name)},
		}

	case reposPaneMRs:
		if s.subMRCursor >= len(s.subMRs) {
			return nil
		}
		mr := s.subMRs[s.subMRCursor]
		var cmds []CommandEntry
		cmds = append(cmds, CommandEntry{Key: "enter", Desc: "checkout branch", Cmd: checkoutRepoMRBranchCmd(git, repo, mr)})
		if mr.WebURL != "" {
			cmds = append(cmds, CommandEntry{Key: "w", Desc: "open URL", Cmd: openBrowserCmd(shell, mr.WebURL)})
		}
		return cmds

	case reposPanePipelines:
		var cmds []CommandEntry
		if s.pipelineCursor < len(s.repoPipelines) {
			p := s.repoPipelines[s.pipelineCursor]
			if p.WebURL != "" {
				cmds = append(cmds, CommandEntry{Key: "enter", Desc: "open URL", Cmd: openBrowserCmd(shell, p.WebURL)})
			}
		}
		if repo.GitLabProjectID != 0 {
			cmds = append(cmds, CommandEntry{Key: "i", Desc: "trigger pipeline", Cmd: openRefSelectCmd(gitlab, repo)})
		}
		return cmds
	}
	return nil
}
