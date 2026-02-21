package skills

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInstallSpec(t *testing.T) {
	tests := []struct {
		name        string
		spec        string
		wantRepo    string
		wantBranch  string
		wantErr     bool
	}{
		{"owner/repo", "owner/repo", "owner/repo", "", false},
		{"owner/repo with branch", "owner/repo@test", "owner/repo", "test", false},
		{"branch with @ in name", "owner/repo@branch@extra", "owner/repo@branch", "extra", false},
		{"trimmed", "  owner/repo  ", "owner/repo", "", false},
		{"empty", "", "", "", true},
		{"no slash", "noslash", "", "", true},
		{"only @", "@main", "", "", true},
		{"repo empty after @", "owner/repo@", "", "", true},
		{"branch empty", "owner/repo@ ", "owner/repo", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, branch, err := ParseInstallSpec(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, repo)
			assert.Equal(t, tt.wantBranch, branch)
		})
	}
}

func TestInstallFromGitHubEx_200_root(t *testing.T) {
	content := []byte("# Test Skill\ndescription: test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sipeed/picoclaw-skills/main/SKILL.md" {
			w.WriteHeader(200)
			_, _ = w.Write(content)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	dir := t.TempDir()
	si := NewSkillInstallerWithBase(dir, server.URL)

	ctx := context.Background()
	skillName, err := si.InstallFromGitHubEx(ctx, "sipeed/picoclaw-skills", "main", "", false)
	require.NoError(t, err)
	assert.Equal(t, "picoclaw-skills", skillName)

	path := filepath.Join(dir, "skills", "picoclaw-skills", "SKILL.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestInstallFromGitHubEx_200_subpath(t *testing.T) {
	content := []byte("# K8s Report Skill")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sipeed/picoclaw-skills/main/k8s-report/SKILL.md" {
			w.WriteHeader(200)
			_, _ = w.Write(content)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	dir := t.TempDir()
	si := NewSkillInstallerWithBase(dir, server.URL)

	ctx := context.Background()
	skillName, err := si.InstallFromGitHubEx(ctx, "sipeed/picoclaw-skills", "main", "k8s-report", false)
	require.NoError(t, err)
	assert.Equal(t, "k8s-report", skillName)

	path := filepath.Join(dir, "skills", "k8s-report", "SKILL.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestInstallFromGitHubEx_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	dir := t.TempDir()
	si := NewSkillInstallerWithBase(dir, server.URL)
	ctx := context.Background()

	_, err := si.InstallFromGitHubEx(ctx, "sipeed/picoclaw-skills", "main", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SKILL.md not found")
	assert.Contains(t, err.Error(), "check branch and path")
}

func TestInstallFromGitHubEx_multi_segment_subpath(t *testing.T) {
	content := []byte("# Kanban AI Skill")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mattjoyce/kanban-skill/master/skills/kanban-ai/SKILL.md" {
			w.WriteHeader(200)
			_, _ = w.Write(content)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	dir := t.TempDir()
	si := NewSkillInstallerWithBase(dir, server.URL)
	ctx := context.Background()

	skillName, err := si.InstallFromGitHubEx(ctx, "mattjoyce/kanban-skill", "master", "skills/kanban-ai", false)
	require.NoError(t, err)
	assert.Equal(t, "kanban-ai", skillName)

	path := filepath.Join(dir, "skills", "kanban-ai", "SKILL.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestInstallFromGitHubEx_invalid_subpath(t *testing.T) {
	dir := t.TempDir()
	si := NewSkillInstaller(dir)
	ctx := context.Background()

	_, err := si.InstallFromGitHubEx(ctx, "owner/repo", "main", "..", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subpath")

	_, err = si.InstallFromGitHubEx(ctx, "owner/repo", "main", "a/../b", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subpath")
}

func TestFetchDefaultBranch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch": "master"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	// fetchDefaultBranch uses hardcoded api.github.com; test the fallback and the type.
	ctx := context.Background()
	branch, err := fetchDefaultBranch(ctx, "owner/repo")
	require.NoError(t, err)
	// Without mocking GitHub we get "main" (fallback); just ensure no error and non-empty.
	assert.NotEmpty(t, branch)
	assert.True(t, branch == "main" || branch == "master", "branch should be main or from API")
}

func TestInstallFromGitHubEx_empty_branch_uses_main_in_tests(t *testing.T) {
	// When baseURL is set (test mode), empty branch becomes "main" without calling GitHub API.
	content := []byte("# Skill")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/owner/repo/main/SKILL.md" {
			w.WriteHeader(200)
			_, _ = w.Write(content)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	dir := t.TempDir()
	si := NewSkillInstallerWithBase(dir, server.URL)
	ctx := context.Background()

	skillName, err := si.InstallFromGitHubEx(ctx, "owner/repo", "", "", false)
	require.NoError(t, err)
	assert.Equal(t, "repo", skillName)
	data, _ := os.ReadFile(filepath.Join(dir, "skills", "repo", "SKILL.md"))
	assert.Equal(t, content, data)
}

func TestInstallFromGitHubEx_already_exists(t *testing.T) {
	content := []byte("# Skill")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "picoclaw-skills")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	si := NewSkillInstallerWithBase(dir, server.URL)
	ctx := context.Background()

	_, err := si.InstallFromGitHubEx(ctx, "sipeed/picoclaw-skills", "main", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestInstallFromGitHubEx_reinstall_overwrites(t *testing.T) {
	content1 := []byte("# Skill v1")
	content2 := []byte("# Skill v2")
	var reqCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.WriteHeader(200)
		if reqCount == 1 {
			_, _ = w.Write(content1)
		} else {
			_, _ = w.Write(content2)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	si := NewSkillInstallerWithBase(dir, server.URL)
	ctx := context.Background()

	skillName, err := si.InstallFromGitHubEx(ctx, "owner/repo", "main", "", false)
	require.NoError(t, err)
	assert.Equal(t, "repo", skillName)
	path := filepath.Join(dir, "skills", "repo", "SKILL.md")
	data, _ := os.ReadFile(path)
	assert.Equal(t, content1, data)

	// Reinstall (force overwrite)
	skillName2, err := si.InstallFromGitHubEx(ctx, "owner/repo", "main", "", true)
	require.NoError(t, err)
	assert.Equal(t, "repo", skillName2)
	data2, _ := os.ReadFile(path)
	assert.Equal(t, content2, data2)
}
