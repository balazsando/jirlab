package service

import (
	"bufio"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ScanRepos walks root looking for directories that contain a .gitlab-ci.yml file.
// For each discovered directory it reads the current git branch and remote URL.
// Hidden directories and known cache/vendor dirs are skipped.
func ScanRepos(root string) ([]Repo, error) {
	var repos []Repo

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}

		if !d.IsDir() {
			return nil
		}

		// Skip hidden and known noise directories
		name := d.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "target" {
			return filepath.SkipDir
		}

		// Check for .gitlab-ci.yml in this directory
		ciFile := filepath.Join(path, ".gitlab-ci.yml")
		if _, statErr := os.Stat(ciFile); statErr != nil {
			return nil // no CI file here, keep walking
		}

		repo := Repo{
			Name:          filepath.Base(path),
			Path:          path,
			CurrentBranch: readGitBranch(path),
			RemoteURL:     readGitRemoteURL(path),
			GitStats:      readGitStats(path),
			Version:       readMavenVersion(path),
		}
		repos = append(repos, repo)

		// Don't descend into a repo — nested repos are rare and slow things down
		return filepath.SkipDir
	})

	return repos, err
}

// readGitBranch returns the current branch name for the git repo at dir,
// or an empty string if dir is not a git repo or git is unavailable.
func readGitBranch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// readGitStats parses `git status --porcelain` and returns staged/unstaged/untracked
// counts, mirroring the information shown by oh-my-zsh's git plugin.
func readGitStats(dir string) GitStats {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return GitStats{}
	}
	var stats GitStats
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		if x == '?' && y == '?' {
			stats.Untracked++
			continue
		}
		if x != ' ' {
			stats.Staged++
		}
		if y == 'M' || y == 'D' || y == 'T' {
			stats.Unstaged++
		}
	}
	return stats
}

// readGitRemoteURL returns the remote origin URL for the git repo at dir.
func readGitRemoteURL(dir string) string {
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RemoteURLToGitLabPath converts a git remote URL to a GitLab path-with-namespace
// if the URL matches the same host as apiBaseURL. Returns "" when no match.
//
// Examples:
//
//	git@gitlab.example.com:mygroup/myrepo.git  →  mygroup/myrepo
//	https://gitlab.example.com/mygroup/myrepo.git  →  mygroup/myrepo
func RemoteURLToGitLabPath(remoteURL, apiBaseURL string) string {
	if remoteURL == "" || apiBaseURL == "" {
		return ""
	}

	apiHost := ""
	if u, err := url.Parse(apiBaseURL); err == nil {
		apiHost = strings.ToLower(u.Hostname())
	}

	// SSH format: git@host:group/repo.git
	if strings.HasPrefix(remoteURL, "git@") {
		// strip "git@"
		rest := remoteURL[len("git@"):]
		// rest is  host:path
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			return ""
		}
		sshHost := strings.ToLower(rest[:colonIdx])
		if apiHost != "" && sshHost != apiHost {
			return ""
		}
		path := strings.TrimPrefix(rest[colonIdx+1:], "/")
		path = strings.TrimSuffix(path, ".git")
		return path
	}

	// HTTPS format
	u, err := url.Parse(remoteURL)
	if err != nil {
		return ""
	}
	if apiHost != "" && strings.ToLower(u.Hostname()) != apiHost {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	return path
}

// ResolveGitLabProjectIDs enriches repos by looking up their GitLab project IDs
// using the remote URL. Repos where the project ID cannot be resolved keep ID=0.
func ResolveGitLabProjectIDs(repos []Repo, client GitLabClient, apiBaseURL string) []Repo {
	result := make([]Repo, len(repos))
	copy(result, repos)
	for i, repo := range result {
		path := RemoteURLToGitLabPath(repo.RemoteURL, apiBaseURL)
		if path == "" {
			continue
		}
		proj, err := client.GetProjectByPath(path)
		if err != nil {
			continue
		}
		result[i].GitLabProjectID = proj.ID
		result[i].DefaultBranch = proj.DefaultBranch
		result[i].WebURL = proj.WebURL
	}
	return result
}

// readMavenVersion reads the Maven revision from {dir}/.mvn/maven.config.
// It looks for a line matching "-Drevision=<value>" and returns the trimmed value.
// Returns an empty string if the file does not exist or contains no revision.
func readMavenVersion(dir string) string {
	configPath := filepath.Join(dir, ".mvn", "maven.config")
	f, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "-Drevision=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "-Drevision="))
		}
	}
	return ""
}
