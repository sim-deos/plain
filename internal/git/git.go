package git

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	Megabyte = 100_000

	ErrNotRepo  = errors.New("not a git repository")
	ErrNotReset = errors.New("must reset")
)

type BranchGraph struct {
	Head    Commit
	Commits map[string]Commit
}

type Header struct {
	Kind string
	Size int64
}

type HeaderScanner struct {
	br *bufio.Reader
}

func NewHeaderScanner(r io.Reader) HeaderScanner {
	return HeaderScanner{br: bufio.NewReader(r)}
}

func (hs *HeaderScanner) Reset(r io.Reader) {
	hs.br.Reset(r)
}

// Scan parses the header from a git object and returns a reader primed to read the following git objects payload.
//
// Scan() can only be called once after either initially creating the HeaderScanner or Resetting it via Reset(). Otherwise,
// an ErrNoReset err will be returned. This is the case because the HeaderScanner type is meant to read a single git object
// at a time.
func (hs *HeaderScanner) Scan() (Header, io.Reader, error) {
	if hs.br.Buffered() > 0 {
		return Header{}, nil, ErrNotReset
	}

	kind, err := hs.br.ReadString(' ')
	if err != nil {
		return Header{}, nil, err
	}
	kind = strings.TrimSuffix(kind, " ")

	sizeStr, err := hs.br.ReadString('\x00')
	if err != nil {
		return Header{}, nil, err
	}
	sizeStr = strings.TrimSuffix(sizeStr, "\x00")

	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return Header{}, nil, err
	}

	return Header{Kind: kind, Size: size}, io.LimitReader(hs.br, size), nil
}

// creater a commit, put it in the commitgraph
type Commit struct {
	Hash, Tree, Message string
	Author, Committer   Signature
	Parents             []string
}

func (c Commit) IsEnd() bool {
	return len(c.Parents) == 0
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

func GetHistoryFor(branch string) (BranchGraph, error) {
	gitDir, err := FindGitDir()
	if err != nil {
		return BranchGraph{}, err
	}

	branchPath := filepath.Join(gitDir, "refs", "heads", branch)
	hcs, err := os.ReadFile(branchPath)
	if err != nil {
		return BranchGraph{}, fmt.Errorf("failed to retrieve head %w", err)
	}

	objectsDir := "objects"

	head := string(hcs[:len(hcs)-1])
	headPath := filepath.Join(gitDir, objectsDir, head[:2], head[2:])

	cb, err := os.ReadFile(headPath)
	if err != nil {
		return BranchGraph{}, fmt.Errorf("failed to read %s: %w", headPath, err)
	}

	cr := bytes.NewReader(cb)
	zr, err := zlib.NewReader(cr)
	if err != nil {
		return BranchGraph{}, fmt.Errorf("failed to create decompression reader: %w", err)
	}
	rz := zr.(zlib.Resetter)
	hs := NewHeaderScanner(zr)
	_, lr, err := hs.Scan()
	if err != nil {
		return BranchGraph{}, err
	}
	pbr := bufio.NewReader(lr)

	headCommit, err := parseCommit(pbr)
	headCommit.Hash = head
	if err != nil {
		return BranchGraph{}, fmt.Errorf("failed to parse head: %w", err)
	}

	graph := BranchGraph{Head: headCommit, Commits: map[string]Commit{head: headCommit}}
	stack := slices.Clone(headCommit.Parents)

	for len(stack) > 0 {
		currCommitHash := stack[len(stack)-1] // get last element
		stack = stack[:len(stack)-1]          // remove it (pop)

		commitPath := filepath.Join(gitDir, objectsDir, currCommitHash[:2], currCommitHash[2:])

		cb, err = os.ReadFile(commitPath)
		if err != nil {
			return BranchGraph{}, err
		}

		// reset readers
		cr.Reset(cb)
		rz.Reset(cr, nil)
		hs.Reset(zr)

		header, lr, err := hs.Scan()
		if err != nil {
			return BranchGraph{}, err
		}

		if header.Kind != "commit" {
			continue
		}

		pbr.Reset(lr)

		commit, err := parseCommit(pbr)
		if err != nil {
			return BranchGraph{}, err
		}
		commit.Hash = currCommitHash

		graph.Commits[currCommitHash] = commit
		stack = append(stack, commit.Parents...)
	}

	return graph, nil
}

// It is assumed that the reader passed into this method is limited via a limited reader
//
// Example commit
// tree 20496db33bfb465582ec5b17ace02cb93598c5f3
// parent 68623a49d7063819550bff568c6e7d78d1c67597
// author Michael Tanami <tanamicodes@gmail.com> 1754691182 -0700
// committer Michael Tanami <tanamicodes@gmail.com> 1754691547 -0700
func parseCommit(br *bufio.Reader) (Commit, error) {
	commit := Commit{}
	for {
		key, err := br.ReadString(' ')
		if err != nil {
			if err == io.EOF {
				return commit, nil
			}
			return Commit{}, err
		}
		key = key[:len(key)-1]

		switch key {
		case "tree":
			val, err := br.ReadString('\n')
			if err != nil {
				return Commit{}, err
			}
			val = val[:len(val)-1]
			commit.Tree = val
		case "parent":
			val, err := br.ReadString('\n')
			if err != nil {
				return Commit{}, err
			}
			val = val[:len(val)-1]
			commit.Parents = append(commit.Parents, val)
		case "author", "committer":
			name, err := br.ReadString('<')
			if err != nil {
				return Commit{}, err
			}
			name = name[:len(name)-2]
			email, err := br.ReadString(' ')
			if err != nil {
				return Commit{}, err
			}
			email = email[:len(email)-2]
			unixTs, err := br.ReadString('\n')
			if err != nil {
				return Commit{}, err
			}
			timestamp, err := parseUnixWithOffset(unixTs[:len(unixTs)-1])
			if err != nil {
				return Commit{}, err
			}
			if key == "author" {
				commit.Author = Signature{
					Name:  name,
					Email: email,
					Time:  timestamp,
				}
			} else {
				commit.Committer = Signature{
					Name:  name,
					Email: email,
					Time:  timestamp,
				}
			}
		case "\n":
			return commit, nil
		}
	}
}

func parseUnixWithOffset(s string) (time.Time, error) {
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid input format")
	}

	// Parse the UNIX timestamp
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid unix timestamp: %w", err)
	}

	// Parse the timezone offset (+/-HHMM)
	if len(parts[1]) != 5 {
		return time.Time{}, fmt.Errorf("invalid offset format")
	}
	sign := 1
	if parts[1][0] == '-' {
		sign = -1
	}
	hours, _ := strconv.Atoi(parts[1][1:3])
	mins, _ := strconv.Atoi(parts[1][3:5])
	offset := sign * (hours*3600 + mins*60)

	loc := time.FixedZone(parts[1], offset)
	return time.Unix(sec, 0).In(loc), nil
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
