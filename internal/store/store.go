// Package store reads and writes tasks as markdown files with YAML frontmatter.
package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// Task is a task, or a post when it lives in the social dir (tk post).
// Platform, Status and URL are for posts only; empty on tasks, so task files
// never show them.
type Task struct {
	ID       string `yaml:"id"`
	Title    string `yaml:"title"`
	Project  string `yaml:"project"`
	Platform string `yaml:"platform,omitempty"`
	Status   string `yaml:"status,omitempty"`
	Deadline string `yaml:"deadline,omitempty"`
	Planned  string `yaml:"planned,omitempty"`
	DoneAt   string `yaml:"done_at,omitempty"`
	URL      string `yaml:"url,omitempty"`
	Created  string `yaml:"created"`

	// Extra keeps frontmatter keys tk doesn't know, so a hand-added field
	// survives the next save. YAML comments are still lost.
	Extra map[string]any `yaml:",inline"`

	Body string `yaml:"-"`
	Path string `yaml:"-"`
}

func (t Task) Done() bool { return t.DoneAt != "" }

type Store struct {
	Dir string
}

// Open uses TK_DIR, falling back to ~/tasks.
func Open() (*Store, error) { return openDir("TK_DIR", "tasks") }

// OpenSocial is the post calendar: TK_SOCIAL_DIR, falling back to ~/social.
// Posts are tasks with a platform, so the same files, lock and day rules apply.
func OpenSocial() (*Store, error) { return openDir("TK_SOCIAL_DIR", "social") }

func openDir(env, name string) (*Store, error) {
	dir := os.Getenv(env)
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) All() ([]Task, error) {
	files, err := filepath.Glob(filepath.Join(s.Dir, "*", "*.md"))
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(files))
	var bad []string
	for _, f := range files {
		t, err := readTask(f)
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", f, err))
			continue
		}
		tasks = append(tasks, t)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	if len(bad) > 0 {
		return tasks, errors.New("unreadable task files:\n  " + strings.Join(bad, "\n  "))
	}
	return tasks, nil
}

var validID = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// Get finds a task by id or unique id prefix. The id is checked first because
// it becomes part of a glob pattern and a path.
func (s *Store) Get(id string) (Task, error) {
	if !validID.MatchString(id) {
		return Task{}, fmt.Errorf("bad task id %q", id)
	}
	matches, _ := filepath.Glob(filepath.Join(s.Dir, "*", id+"-*.md"))
	if len(matches) == 0 {
		matches, _ = filepath.Glob(filepath.Join(s.Dir, "*", id+"*.md"))
	}
	switch len(matches) {
	case 0:
		return Task{}, fmt.Errorf("no task %q", id)
	case 1:
		return readTask(matches[0])
	default:
		return Task{}, fmt.Errorf("id %q matches %d tasks, use more characters", id, len(matches))
	}
}

// lock serialises writers across processes: the TUI, the CLI and Claude all
// write the same folder. Reads stay lock-free; every write re-reads under it.
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// Add creates a task with just a title and project.
func (s *Store) Add(title, project string) (Task, error) {
	return s.Create(Task{Title: title, Project: project})
}

// Create assigns an id and writes a new task. Validation happens before any
// file exists, so a failed create leaves nothing to clean up.
func (s *Store) Create(t Task) (Task, error) {
	t.Title = strings.TrimSpace(t.Title)
	if t.Title == "" {
		return Task{}, errors.New("title is empty")
	}
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	t.ID, t.Path = s.newID(), ""
	if t.Created == "" {
		t.Created = Today()
	}
	return t, s.save(&t)
}

// Update re-reads the task under the lock, applies fn and saves. fn sees the
// file as it is now, so a change made by another process is never undone.
func (s *Store) Update(id string, fn func(*Task) error) (Task, error) {
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	t, err := s.Get(id)
	if err != nil {
		return Task{}, err
	}
	if err := fn(&t); err != nil {
		return Task{}, err
	}
	return t, s.save(&t)
}

// Save writes t as given. Prefer Update, which cannot overwrite newer data.
func (s *Store) Save(t *Task) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return s.save(t)
}

func (s *Store) Delete(id string) (Task, error) {
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	t, err := s.Get(id)
	if err != nil {
		return Task{}, err
	}
	if err := os.Remove(t.Path); err != nil {
		return Task{}, err
	}
	os.Remove(filepath.Dir(t.Path)) // only succeeds when the project dir is now empty
	return t, nil
}

var validProject = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// newID is base36 seconds since 2024, short enough to type, sortable by
// creation. Callers hold the lock, so the existence check cannot race.
func (s *Store) newID() string {
	n := time.Now().Unix() - 1704067200
	for {
		id := strconv.FormatInt(n, 36)
		if m, _ := filepath.Glob(filepath.Join(s.Dir, "*", id+"-*.md")); len(m) == 0 {
			return id
		}
		n++
	}
}

// save writes atomically and renames the file when title or project changed.
// Callers hold the lock.
func (s *Store) save(t *Task) error {
	// macOS folders are case-insensitive: "Work" and "work" are one folder, and
	// a case-only rename used to delete the file it had just written.
	t.Project = strings.ToLower(strings.TrimSpace(t.Project))
	if !validProject.MatchString(t.Project) {
		return fmt.Errorf("project %q: use letters, digits, - _ .", t.Project)
	}
	dir := filepath.Join(s.Dir, t.Project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, t.ID+"-"+slug(t.Title)+".md")

	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(t); err != nil {
		return err
	}
	buf.WriteString("---\n")
	if body := trimBody(t.Body); body != "" {
		buf.WriteString(body + "\n")
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(buf.Bytes())
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if t.Path != "" && t.Path != path && !sameFile(t.Path, path) {
		os.Remove(t.Path)
		os.Remove(filepath.Dir(t.Path)) // only succeeds when the old project dir is now empty
	}
	t.Path = path
	return nil
}

func sameFile(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

// trimBody drops surrounding blank lines and trailing space but keeps the
// first line's indentation, which matters for code blocks.
func trimBody(b string) string {
	b = strings.TrimRight(b, " \t\n")
	for {
		i := strings.IndexByte(b, '\n')
		if i < 0 || strings.TrimSpace(b[:i]) != "" {
			break
		}
		b = b[i+1:]
	}
	if strings.TrimSpace(b) == "" {
		return ""
	}
	return b
}

func readTask(path string) (Task, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Task{}, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Task{}, errors.New("missing frontmatter")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return Task{}, errors.New("unclosed frontmatter")
	}
	var t Task
	if err := yaml.Unmarshal([]byte(text[4:4+end]), &t); err != nil {
		return Task{}, err
	}
	body := text[4+end+4:]
	t.Body = trimBody(body)
	t.Path = path
	if t.ID == "" {
		return Task{}, errors.New("missing id")
	}
	if t.Project == "" {
		t.Project = filepath.Base(filepath.Dir(path))
	}
	return t, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Polish titles are common (posts); without this "książki" becomes "ksi-ki".
var polish = strings.NewReplacer("ą", "a", "ć", "c", "ę", "e", "ł", "l", "ń", "n", "ó", "o", "ś", "s", "ź", "z", "ż", "z")

func slug(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(polish.Replace(strings.ToLower(s)), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	if s == "" {
		s = "task"
	}
	return s
}
