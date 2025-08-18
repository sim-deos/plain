package git

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrNotRepo = errors.New("not a git repository")
)

type Commit struct {
	Hash      string
	Tree      string
	Parents   []*Commit
	Author    Signature
	Committer Signature
	Message   string
}

type Signature struct {
	Name  string
	Email string
	Time  time.Time
}

type Client interface {
	Init() error
	IsBranchDirty() (bool, error)
	GetCurrentBranch() (string, error)

	// Create a new branch off of the 'from' branch. If from == 'here', will get the current
	// branch and base the new branch off of it.
	CreateBranch(name, from string) error
	SwitchBranch(name string) error
}

func FindGitDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		git := filepath.Join(cwd, ".git")
		info, err := os.Stat(git)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				up := filepath.Dir(cwd)
				if up == cwd {
					return "", ErrNotRepo
				}
				cwd = up
				continue
			}
			return "", err
		}

		if info.IsDir() {
			return git, nil
		}

		fb, err := os.ReadFile(git)
		if err != nil {
			return "", err
		}

		git = strings.TrimSpace(strings.TrimPrefix(string(fb), "gitdir: "))
		return filepath.Abs(git)
	}
}

func GetHistoryFor(branch string) (Commit, error) {
	gitDir, err := FindGitDir()
	if err != nil {
		return Commit{}, err
	}

	branchPath := filepath.Join(gitDir, "refs", "heads", branch)
	hs, err := os.ReadFile(branchPath)
	if err != nil {
		return Commit{}, fmt.Errorf("failed to retrieve head %w", err)
	}

	head := string(hs[:len(hs)-1])
	commitFilePath := filepath.Join(gitDir, "objects", head[:2], head[2:])

	cb, err := os.ReadFile(commitFilePath)
	if err != nil {
		return Commit{}, fmt.Errorf("failed to read %s: %w", commitFilePath, err)
	}

	cr := bytes.NewReader(cb)
	zr, err := zlib.NewReader(cr)
	if err != nil {
		return Commit{}, fmt.Errorf("failed to create decompression reader: %w", err)
	}
	rz := zr.(zlib.Resetter)
	br := bufio.NewReader(zr)

	// commit 236tree 20496db33bfb465582ec5b17ace02cb93598c5f3
	// parent 68623a49d7063819550bff568c6e7d78d1c67597
	// author Michael Tanami <tanamicodes@gmail.com> 1754691182 -0700
	// committer Michael Tanami <tanamicodes@gmail.com> 1754691547 -0700

	// moved to DI

	stack := [][]byte{cb}
	for len(stack) > 0 {
		cb = stack[len(stack)-1]     // get last element
		stack = stack[:len(stack)-1] // remove it (pop)

		// reset readers
		cr.Reset(cb)
		rz.Reset(cr, nil)
		br.Reset(zr)

		kind, err := br.ReadString(' ')
		if err != nil {
			return Commit{}, err
		}
		kind = kind[:len(kind)-1]

		if kind != "commit" {
			continue
		}

		header, _ := br.ReadBytes('\x00')
		fmt.Println(string(header))
	}

	return Commit{}, nil
}

type ShellClient struct{}

func NewShellClient() *ShellClient {
	return &ShellClient{}
}

func (c *ShellClient) Init() error {
	gitCmd := exec.Command("git", "init")

	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr

	return gitCmd.Run()
}

func (c *ShellClient) IsBranchDirty() (bool, error) {
	gitCmd := exec.Command("git", "diff", "--quiet", "--ignore-submodules", "HEAD")
	_, err := gitCmd.Output()
	code := err.Error()

	if code == "exit status 1" {
		return true, nil
	}

	if code == "exit status 0" {
		return false, nil
	}

	return true, err
}

func (c *ShellClient) GetCurrentBranch() (string, error) {
	gitBranchCmd := exec.Command("git", "branch", "--show-current")

	output, err := gitBranchCmd.Output()
	if err != nil {
		return "", err
	}

	output = output[:len(output)-1] // trim trailing newline

	return string(output), nil
}

func (c *ShellClient) CreateBranch(name, from string) error {
	gitArgs := []string{"checkout", "-b", name}

	if from == "here" {
		currentBranch, err := c.GetCurrentBranch()
		if err != nil {
			return err
		}
		from = currentBranch
	}

	gitArgs = append(gitArgs, from)

	gitCmd := exec.Command("git", gitArgs...)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr

	return gitCmd.Run()
}

func (c *ShellClient) SwitchBranch(name string) error {
	fmt.Println("switch not yet implemented")
	return nil
}
