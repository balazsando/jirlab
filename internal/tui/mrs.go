package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andob/jirlab/internal/integration"
	"github.com/andob/jirlab/internal/service"
)

// mrsLoadedMsg is delivered when the MR fetch completes.
type mrsLoadedMsg struct {
	mrs []service.MergeRequest
	err error
}

// mrsActionDoneMsg is delivered after a merge or close action completes.
type mrsActionDoneMsg struct {
	message string
	reload  bool
}

// allPipelinesLoadedMsg is delivered when active pipelines across all repos are fetched.
type allPipelinesLoadedMsg struct {
	pipelines []service.Pipeline
	err       error
}

// mrsPaneTab selects which pane the MRs section shows.
type mrsPaneTab int

const (
	mrsPaneMRs mrsPaneTab = iota
	mrsPanePipelines
)

// fetchAllPipelinesCmd fetches active (running/pending) pipelines for all known projects.
func fetchAllPipelinesCmd(gitlab integration.GitLabService, projectIDs []int, repoNames map[int]string) tea.Cmd {
	return func() tea.Msg {
		if gitlab == nil || len(projectIDs) == 0 {
			return allPipelinesLoadedMsg{}
		}
		pipelines, err := gitlab.GetAllActivePipelines(projectIDs, repoNames)
		return allPipelinesLoadedMsg{pipelines: pipelines, err: err}
	}
}

// fetchMRsCmd loads open MRs — all MRs for locally known projects plus user-scoped ones.
func fetchMRsCmd(gitlab integration.GitLabService, projectIDs []int) tea.Cmd {
	return func() tea.Msg {
		var mrs []service.MergeRequest
		var err error
		if len(projectIDs) > 0 {
			mrs, err = gitlab.GetAllMRsForProjects(projectIDs)
		} else {
			mrs, err = gitlab.GetMergeRequests()
		}
		if err != nil {
			return mrsLoadedMsg{err: err}
		}
		return mrsLoadedMsg{mrs: mrs}
	}
}

// MRsSection is the Merge Requests tab.
type MRsSection struct {
	mrs        []service.MergeRequest
	cursor     int
	loading    bool
	statusMsg  string
	gitlab     integration.GitLabService
	git        integration.GitCmdService
	fs         integration.FilesystemService
	shell      integration.ShellService
	repos      []service.Repo // local repos for branch checkout and patch download
	projectIDs []int          // resolved GitLab project IDs for local repos
	repoNames  map[int]string // projectID → repo name
	termHeight int            // terminal height, set on resize

	// Pipelines subtab
	activePane       mrsPaneTab
	allPipelines     []service.Pipeline
	pipelinesLoading bool
	pipelinesErr     string
	pipelineCursor   int
}

func newMRsSection(gitlab integration.GitLabService, git integration.GitCmdService, fs integration.FilesystemService, shell integration.ShellService) MRsSection {
	return MRsSection{
		loading:    true,
		gitlab:     gitlab,
		git:        git,
		fs:         fs,
		shell:      shell,
		activePane: mrsPaneMRs,
		repoNames:  make(map[int]string),
	}
}

func (s MRsSection) update(msg tea.Msg) (MRsSection, tea.Cmd) {
	switch msg := msg.(type) {
	case mrsLoadedMsg:
		s.loading = false
		if msg.err != nil {
			return s, func() tea.Msg { return errMsg{source: "mrs", err: msg.err} }
		}
		s.mrs = msg.mrs
		s.cursor = 0
		if s.gitlab != nil && len(s.projectIDs) > 0 {
			s.pipelinesLoading = true
			return s, fetchAllPipelinesCmd(s.gitlab, s.projectIDs, s.repoNames)
		}

	case mrsActionDoneMsg:
		s.statusMsg = msg.message
		if msg.reload && s.gitlab != nil {
			s.loading = true
			return s, fetchMRsCmd(s.gitlab, s.projectIDs)
		}

	case allPipelinesLoadedMsg:
		s.pipelinesLoading = false
		if msg.err != nil {
			s.pipelinesErr = msg.err.Error()
		} else {
			s.pipelinesErr = ""
			// keep only active pipelines
			var active []service.Pipeline
			for _, p := range msg.pipelines {
				if p.Status == "running" || p.Status == "pending" {
					active = append(active, p)
				}
			}
			s.allPipelines = active
			s.pipelineCursor = 0
		}

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

func (s MRsSection) handleKey(msg tea.KeyMsg) (MRsSection, tea.Cmd) {
	switch msg.String() {
	case "tab":
		if s.activePane == mrsPaneMRs {
			// Only switch to pipelines pane if there is something to show
			if len(s.allPipelines) > 0 || s.pipelinesLoading {
				s.activePane = mrsPanePipelines
			}
		} else {
			s.activePane = mrsPaneMRs
		}
		return s, nil

	case "j", "down":
		if s.activePane == mrsPanePipelines {
			if s.pipelineCursor < len(s.allPipelines)-1 {
				s.pipelineCursor++
			}
			return s, nil
		}
		if s.cursor < len(s.mrs)-1 {
			s.cursor++
		}
	case "k", "up":
		if s.activePane == mrsPanePipelines {
			if s.pipelineCursor > 0 {
				s.pipelineCursor--
			}
			return s, nil
		}
		if s.cursor > 0 {
			s.cursor--
		}
	case "d":
		if mr := s.selected(); mr != nil {
			pID, iid, title := mr.ProjectID, mr.IID, mr.Title
			glab := s.gitlab
			return s, func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message: fmt.Sprintf("Close MR: %s?", truncStr(title, 50)),
					onConfirm: func() tea.Msg {
						if glab != nil && pID != 0 {
							if err := glab.CloseMR(pID, iid); err != nil {
								return errMsg{source: "mrs", err: err}
							}
						}
						return mrsActionDoneMsg{message: "MR closed", reload: true}
					},
				}}
			}
		}
	case "c": // checkout MR branch locally
		if mr := s.selected(); mr != nil {
			repos := s.repos
			mrCopy := *mr
			git := s.git
			return s, func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message: fmt.Sprintf("Checkout branch %s?", mrCopy.SourceBranch),
					onConfirm: func() tea.Msg {
						return checkoutMRBranchCmd(git, repos, mrCopy)()
					},
				}}
			}
		}

	case "enter", "w":
		if s.activePane == mrsPanePipelines {
			if s.pipelineCursor < len(s.allPipelines) {
				p := s.allPipelines[s.pipelineCursor]
				if p.WebURL != "" {
					return s, openBrowserCmd(s.shell, p.WebURL)
				}
			}
			return s, nil
		}
		if mr := s.selected(); mr != nil && mr.WebURL != "" {
			return s, openBrowserCmd(s.shell, mr.WebURL)
		}
	case "p": // download patch file to repo/.mr/<issue>.patch
		if mr := s.selected(); mr != nil && mr.WebURL != "" && s.gitlab != nil {
			mrCopy := *mr
			repos := s.repos
			glab := s.gitlab
			fs := s.fs
			return s, downloadMRPatchCmd(glab, fs, mrCopy, repos)
		}
	case "m":
		if mr := s.selected(); mr != nil && mr.IsMine {
			pID, iid, title := mr.ProjectID, mr.IID, mr.Title
			glab := s.gitlab
			return s, func() tea.Msg {
				return OpenModalMsg{M: ConfirmModal{
					message: fmt.Sprintf("Merge MR: %s?", truncStr(title, 50)),
					onConfirm: func() tea.Msg {
						if glab != nil && pID != 0 {
							if err := glab.MergeMR(pID, iid); err != nil {
								return errMsg{source: "mrs", err: err}
							}
						}
						return mrsActionDoneMsg{message: "MR merged", reload: true}
					},
				}}
			}
		} else {
			s.statusMsg = "Can only merge your own MRs"
		}

	case "right":
		cmds := s.buildCommandPalette()
		if len(cmds) > 0 {
			mrsH := s.termHeight / 2
			if mrsH < 4 {
				mrsH = 4
			}
			var anchorY int
			if s.activePane == mrsPanePipelines {
				anchorY = mrsH + 4 + s.pipelineCursor
			} else {
				anchorY = 4 + s.cursor
			}
			return s, func() tea.Msg {
				return OpenModalMsg{M: CommandPaletteModal{commands: cmds, AnchorY: anchorY}}
			}
		}
	}
	return s, nil
}

// buildCommandPalette returns context-sensitive actions for the active MRs pane.
func (s MRsSection) buildCommandPalette() []CommandEntry {
	if s.activePane == mrsPanePipelines {
		if s.pipelineCursor < len(s.allPipelines) {
			p := s.allPipelines[s.pipelineCursor]
			if p.WebURL != "" {
				return []CommandEntry{
					{Key: "enter/w", Desc: "open URL", Cmd: openBrowserCmd(s.shell, p.WebURL)},
				}
			}
		}
		return nil
	}
	// MRs main pane
	mr := s.selected()
	if mr == nil {
		return nil
	}
	var entries []CommandEntry
	if mr.WebURL != "" {
		entries = append(entries, CommandEntry{Key: "enter/w", Desc: "open URL", Cmd: openBrowserCmd(s.shell, mr.WebURL)})
	}
	if mr.WebURL != "" && s.gitlab != nil {
		mrCopy := *mr
		repos := s.repos
		glab := s.gitlab
		fs := s.fs
		entries = append(entries, CommandEntry{Key: "p", Desc: "patch file", Cmd: downloadMRPatchCmd(glab, fs, mrCopy, repos)})
	}
	{
		mrCopy := *mr
		repos := s.repos
		git := s.git
		entries = append(entries, CommandEntry{Key: "c", Desc: "checkout", Cmd: func() tea.Msg {
			return OpenModalMsg{M: ConfirmModal{
				message: fmt.Sprintf("Checkout branch %s?", mrCopy.SourceBranch),
				onConfirm: func() tea.Msg {
					return checkoutMRBranchCmd(git, repos, mrCopy)()
				},
			}}
		}})
	}
	if mr.IsMine {
		pID, iid, title := mr.ProjectID, mr.IID, mr.Title
		glab := s.gitlab
		entries = append(entries, CommandEntry{Key: "m", Desc: "merge", Cmd: func() tea.Msg {
			return OpenModalMsg{M: ConfirmModal{
				message: fmt.Sprintf("Merge MR: %s?", truncStr(title, 50)),
				onConfirm: func() tea.Msg {
					if glab != nil && pID != 0 {
						if err := glab.MergeMR(pID, iid); err != nil {
							return errMsg{source: "mrs", err: err}
						}
					}
					return mrsActionDoneMsg{message: "MR merged", reload: true}
				},
			}}
		}})
		entries = append(entries, CommandEntry{Key: "d", Desc: "close", Cmd: func() tea.Msg {
			return OpenModalMsg{M: ConfirmModal{
				message: fmt.Sprintf("Close MR: %s?", truncStr(title, 50)),
				onConfirm: func() tea.Msg {
					if glab != nil && pID != 0 {
						if err := glab.CloseMR(pID, iid); err != nil {
							return errMsg{source: "mrs", err: err}
						}
					}
					return mrsActionDoneMsg{message: "MR closed", reload: true}
				},
			}}
		}})
	}
	return entries
}

func (s *MRsSection) selected() *service.MergeRequest {
	if len(s.mrs) == 0 || s.cursor >= len(s.mrs) {
		return nil
	}
	mr := s.mrs[s.cursor]
	return &mr
}

func (s MRsSection) view(width, height int) string {
	mrsH := height / 2
	if mrsH < 4 {
		mrsH = 4
	}
	// overhead: sectionSepLine(1) + statusMsg(1) = 2
	pipeH := height - mrsH - 2
	if pipeH < 3 {
		pipeH = 3
	}

	upper := s.viewMRsPane(width, mrsH)
	sep := sectionSepLine(width)
	lower := s.viewPipelinesPane(width, pipeH)

	parts := []string{upper, sep, lower}
	if s.statusMsg != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render("  "+s.statusMsg))
	}
	return strings.Join(parts, "\n")
}

func (s MRsSection) viewMRsPane(width, height int) string {
	if p := subPanePlaceholder(s.loading, len(s.mrs), "Loading merge requests…", "", "No open merge requests assigned to you."); p != "" {
		return p
	}

	authorW := 18
	statusW := 10
	titleW := width - authorW - statusW - 6
	repoW := 22
	if titleW > 40 {
		repoW = 22
		titleW = width - authorW - statusW - repoW - 8
	}
	if titleW < 15 {
		titleW = 15
	}

	colFmt := func(str string, w int) string {
		return columnHeaderStyle.Width(w).Render(truncStr(str, w))
	}
	header := colFmt("REPOSITORY", repoW) + " " + colFmt("TITLE", titleW) + " " + colFmt("AUTHOR", authorW) + " " + colFmt("STATUS", statusW)

	maxRows := height - 4
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.cursor, len(s.mrs), maxRows)

	var rows []string
	rows = append(rows, header, tableHeaderSepLine(width))
	for i := start; i < end; i++ {
		mr := s.mrs[i]
		color := MRColor(mr)
		repoName := mr.Repository
		if idx := strings.LastIndex(repoName, "/"); idx >= 0 {
			repoName = repoName[idx+1:]
		}
		line := fmt.Sprintf("%-*s %-*s %-*s %-*s",
			repoW, truncStr(repoName, repoW),
			titleW, truncStr(mr.Title, titleW),
			authorW, truncStr(mr.Author, authorW),
			statusW, truncStr(mr.Status, statusW),
		)
		var style lipgloss.Style
		if i == s.cursor && s.activePane == mrsPaneMRs {
			style = selectedRowStyle.Foreground(color).Width(width)
		} else if i == s.cursor {
			style = selectedRowDimStyle.Foreground(color).Width(width)
		} else {
			style = normalRowStyle.Foreground(color).Width(width)
		}
		rows = append(rows, style.Render(line))
	}
	rows = append(rows, tableHeaderSepLine(width))
	rows = append(rows, MRColorLegendBar(width))
	rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Render(
		"  p: patch file  c: checkout branch  m: merge MR  d: close MR  →: commands",
	))
	return strings.Join(rows, "\n")
}

func (s MRsSection) viewPipelinesPane(width, height int) string {
	if p := subPanePlaceholder(s.pipelinesLoading, len(s.allPipelines), "Loading active pipelines…", s.pipelinesErr, "No active pipelines (running/pending) across all repos"); p != "" {
		return p
	}

	repoW := 20
	statusW := 10
	jobsW := 6
	ageW := 8
	refW := width - repoW - statusW - jobsW - ageW - 10
	if refW < 15 {
		refW = 15
	}

	colH := func(str string, w int) string { return columnHeaderStyle.Width(w).Render(truncStr(str, w)) }
	header := colH("REPO", repoW) + " " + colH("REF", refW) + " " + colH("STATUS", statusW) + " " + colH("JOBS", jobsW) + " " + colH("AGE", ageW)

	maxRows := height - 4
	if maxRows < 1 {
		maxRows = 1
	}
	start, end := scrollWindow(s.pipelineCursor, len(s.allPipelines), maxRows)
	now := time.Now()

	var rows []string
	rows = append(rows, header, tableHeaderSepLine(width))
	for i := start; i < end; i++ {
		p := s.allPipelines[i]
		age := "-"
		if !p.CreatedAt.IsZero() {
			age = kubeAge(p.CreatedAt.Format(time.RFC3339), now)
		}
		jobs := "-"
		if p.JobsTotal > 0 {
			jobs = fmt.Sprintf("%d/%d", p.JobsSuccess, p.JobsTotal)
		}
		line := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s",
			repoW, truncStr(p.RepoName, repoW),
			refW, truncStr(p.Ref, refW),
			statusW, truncStr(p.Status, statusW),
			jobsW, jobs,
			ageW, age,
		)
		col := pipelineStatusColor(p.Status)
		if i == s.pipelineCursor && s.activePane == mrsPanePipelines {
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
		"  enter/w: open pipeline  →: commands",
	))
	return strings.Join(rows, "\n")
}

func (s MRsSection) helpKeys() []HelpEntry { return MRsKeys }

// downloadMRPatchCmd downloads a merge request's .patch file and saves it to
// {repoPath}/.mr/{issueKey}.patch (falls back to mr-{IID}.patch when no issue key).
func downloadMRPatchCmd(gitlab integration.GitLabService, fs integration.FilesystemService, mr service.MergeRequest, repos []service.Repo) tea.Cmd {
	return func() tea.Msg {
		// Find the local repo path that matches this MR's project.
		repoPath := ""
		for _, r := range repos {
			if r.GitLabProjectID == mr.ProjectID {
				repoPath = r.Path
				break
			}
		}
		if repoPath == "" {
			return errMsg{source: "mrs", err: fmt.Errorf("no local repo for project %d — checkout the repo first", mr.ProjectID)}
		}

		data, err := gitlab.DownloadMRPatch(mr.ProjectID, mr.IID)
		if err != nil {
			return errMsg{source: "mrs", err: fmt.Errorf("download patch: %w", err)}
		}

		filename := mr.IssueKey
		if filename == "" {
			filename = fmt.Sprintf("mr-%d", mr.IID)
		}
		patchPath := filepath.Join(repoPath, ".mr", filename+".patch")
		if err := fs.SaveFile(patchPath, data); err != nil {
			return errMsg{source: "mrs", err: fmt.Errorf("write patch: %w", err)}
		}

		return mrsActionDoneMsg{message: fmt.Sprintf("Patch saved → %s", patchPath)}
	}
}
