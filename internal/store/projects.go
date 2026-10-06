package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Project struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// Projects is every task folder plus every entry in projects.yaml. A project
// without a configured path resolves to ~/dev/<name> when that folder exists.
func (s *Store) Projects() []Project {
	paths := s.projectPaths()
	names := map[string]bool{}
	for n := range paths {
		names[n] = true
	}
	entries, _ := os.ReadDir(s.Dir)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names[e.Name()] = true
		}
	}
	out := make([]Project, 0, len(names))
	for n := range names {
		out = append(out, Project{Name: n, Path: s.resolvePath(n, paths)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) RepoPath(project string) string {
	return s.resolvePath(project, s.projectPaths())
}

func (s *Store) resolvePath(name string, paths map[string]string) string {
	home, _ := os.UserHomeDir()
	if p := paths[name]; p != "" {
		if strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		}
		return p
	}
	guess := filepath.Join(home, "dev", name)
	if st, err := os.Stat(guess); err == nil && st.IsDir() {
		return guess
	}
	return ""
}

func (s *Store) projectPaths() map[string]string {
	m := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(s.Dir, "projects.yaml"))
	if err == nil {
		yaml.Unmarshal(raw, &m)
	}
	return m
}
