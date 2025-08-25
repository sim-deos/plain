package git

import (
	"bufio"
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
	ErrNotRepo  = errors.New("not a git repository")
	ErrNotReset = errors.New("must reset")
)

// GitObjectKind represents the type of object stored in a Git repository.
//
// Git defines four fundamental object kinds: commit, tree, blob, and tag.
type GitObjectKind int

const (
	// CommitObject identifies a Git commit object,
	// which records a snapshot of the repository state.
	CommitObject GitObjectKind = iota

	// TreeObject identifies a Git tree object,
	// which represents a directory and its contents.
	TreeObject

	// BlobObject identifies a Git blob object,
	// which stores the raw file data.
	BlobObject

	// TagObject identifies a Git tag object,
	// which attaches a human-readable name to another object.
	TagObject
)

// BranchHistory represents the history of a git branch in a users repository.
//
// It holds the head commit and a DAG representation of the rest of the branches history.
type BranchHistory struct {
	Head  Commit            // The head of the branch
	Graph map[string]Commit // A DAG in the form of an adjacecny list to access the rest of the branches history.
}

// ObjectHeader represents the header of a git objects file.
type ObjectHeader struct {
	Kind GitObjectKind // The kind of git object object is
	Size int64         // the size of the object in bytes
}

// Commit represents a git commit object.
type Commit struct {
	Hash      string    // The commit hash belonging to this commit
	Tree      string    // The tree this commit object is a part of
	Message   string    // The message given when this commit was comitted
	Author    Signature // The author of the commit
	Committer Signature // The committer of this commit
	Parents   []string  // This commits parents
}

// Returns the commits display name (the first 7 characters of the commits hash)
func (c Commit) DisName() string {
	return c.Hash[:7]
}

// Returns true if this commit is a leaf (has no parents), otherwise it returns false.
func (c Commit) IsLeaf() bool {
	return len(c.Parents) == 0
}

// Represents the aignature on a commit.
//
// Git signatures are made up of the name and email of the committer as well as the time that the commit was committed.
type Signature struct {
	Name  string    // The committers name
	Email string    // The committers email
	Time  time.Time // The time the commit was committed
}

// Decoder reads, decompresses, and parses git files to return usable git objects.
//
// A new Decoder is created by calling [NewDecoder].
// The same instance of a Deocder can be used to decode many git objects by resetting the Deocder after each use.
type Decoder struct {
	zr io.ReadCloser
	br *bufio.Reader
}

// Creates a new Decoder.
// Initialization will fail if the given source is not zlib compressed.
//
// Be sure to defer closing this instance.
func NewDecoder(src io.Reader) (*Decoder, error) {
	z, err := zlib.NewReader(src)
	if err != nil {
		return &Decoder{}, err
	}

	return &Decoder{
		zr: z,
		br: bufio.NewReader(z),
	}, nil
}

// Reset resets all internal state and primes the decoder to start reading from src.
// Will fail is src is not zlib compressed
func (d *Decoder) Reset(src io.Reader) error {
	err := d.zr.(zlib.Resetter).Reset(src, nil)
	if err != nil {
		return err
	}
	d.br.Reset(d.zr)
	return nil
}

// Closes the Decoder.
func (d *Decoder) Close() error {
	return d.zr.Close()
}

// Reads the header of the current git object.
// Header contains the objects kind and size in bytes.
//
// If the git object itself was already decoded, than this method will fail unless the Decoder is reset.
func (d *Decoder) Header() (ObjectHeader, error) {
	if d.br.Buffered() > 0 {
		return ObjectHeader{}, ErrNotReset
	}

	kindStr, err := d.br.ReadString(' ')
	if err != nil {
		return ObjectHeader{}, err
	}
	kindStr = strings.TrimSuffix(kindStr, " ")

	var kind GitObjectKind
	switch kindStr {
	case "commit":
		kind = CommitObject
	case "tree":
		kind = TreeObject
	case "blob":
		kind = BlobObject
	case "tag":
		kind = TagObject
	}

	sizeStr, err := d.br.ReadString('\x00')
	if err != nil {
		return ObjectHeader{}, err
	}
	sizeStr = strings.TrimSuffix(sizeStr, "\x00")

	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return ObjectHeader{}, err
	}

	return ObjectHeader{Kind: kind, Size: size}, nil
}

// Reads, decodes, and returns the current git object.
// Once this method is called, Header() will result in an error.
func (d *Decoder) DecodeCommit(hash string) (Commit, error) {
	br := d.br
	commit := Commit{Hash: hash}
	for {
		lineBytes, err := br.ReadSlice('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return Commit{}, err
		}
		lineBytes = lineBytes[:len(lineBytes)-1]

		if len(lineBytes) == 0 {
			break
		}

		sepIndex := slices.Index(lineBytes, ' ')
		if sepIndex == -1 {
			return Commit{}, fmt.Errorf("parse: line did not contain canonical separator")
		}

		header := string(lineBytes[:sepIndex])
		value := lineBytes[sepIndex+1:]
		switch header {
		case "tree":
			commit.Tree = string(value)
		case "parent":
			commit.Parents = append(commit.Parents, string(value))
		case "author", "committer":
			emailStartIndex := slices.Index(value, '<')
			emailEndIndex := slices.Index(value, '>')
			name := string(value[:emailStartIndex-1])
			email := string(value[emailStartIndex+1 : emailEndIndex])
			unixTs := string(value[emailEndIndex+2:])

			colloquialTs, err := parseUnixWithOffset(unixTs)
			if err != nil {
				return Commit{}, fmt.Errorf("parse: failed to parse commit due to time error %w", err)
			}

			sig := Signature{
				Name:  name,
				Email: email,
				Time:  colloquialTs,
			}

			if header == "author" {
				commit.Author = sig
			} else {
				commit.Committer = sig
			}
		}
	}

	message, err := io.ReadAll(br)
	if err != nil {
		return Commit{}, fmt.Errorf("parse: failed to parse commit message for %s. %w", hash, err)
	}
	commit.Message = strings.TrimSuffix(string(message), "\n")
	return commit, nil
}

// Returns a path to the .git directory in this repo.
// Will return an error of called from outside a git repository.
func FindGitDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	var gitDir string
	var info fs.FileInfo
	for {
		gitDir = filepath.Join(cwd, ".git")
		info, err = os.Stat(gitDir)

		if err == nil {
			break
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		up := filepath.Dir(cwd)
		if up == cwd {
			return "", ErrNotRepo
		}
		cwd = up
	}

	if !info.IsDir() {
		fileBytes, err := os.ReadFile(gitDir)
		if err != nil {
			return "", err
		}
		gitDir = strings.TrimSpace(strings.TrimPrefix(string(fileBytes), "gitdir: "))
	}

	return filepath.Abs(gitDir)
}

// Get the [BranchHistory] for the given branch.
func GetHistoryFor(branch string) (BranchHistory, error) {
	gitDir, err := FindGitDir()
	if err != nil {
		return BranchHistory{}, err
	}

	branchPath := filepath.Join(gitDir, "refs", "heads", branch)
	headBytes, err := os.ReadFile(branchPath)
	if err != nil {
		return BranchHistory{}, err
	}

	headCommitStr := strings.TrimSpace(string(headBytes))
	objectsPath := filepath.Join(gitDir, "objects")
	headCommitPath := filepath.Join(objectsPath, headCommitStr[:2], headCommitStr[2:])

	objStream, _ := os.Open(headCommitPath)
	d, err := NewDecoder(objStream)
	if err != nil {
		return BranchHistory{}, fmt.Errorf("git: failed to init object decoder: %w", err)
	}
	defer d.Close()

	header, err := d.Header()
	if err != nil {
		return BranchHistory{}, fmt.Errorf("git: failed to parse header: %w", err)
	}

	if header.Kind != CommitObject {
		return BranchHistory{}, errors.New("start file not a commit")
	}

	headCommitObj, err := d.DecodeCommit(headCommitStr)
	if err != nil {
		return BranchHistory{}, fmt.Errorf("failed to parse head: %w", err)
	}

	graph := BranchHistory{Head: headCommitObj, Graph: map[string]Commit{headCommitStr: headCommitObj}}
	stack := slices.Clone(headCommitObj.Parents)
	for len(stack) > 0 {
		currCommitHash := stack[len(stack)-1] // get last element
		stack = stack[:len(stack)-1]          // remove it (pop)

		commitPath := filepath.Join(objectsPath, currCommitHash[:2], currCommitHash[2:])

		objStream, err = os.Open(commitPath)
		if err != nil {
			return BranchHistory{}, err
		}

		d.Reset(objStream)
		header, err := d.Header()
		if err != nil {
			return BranchHistory{}, err
		}

		if header.Kind != CommitObject {
			continue
		}

		commit, err := d.DecodeCommit(currCommitHash)
		if err != nil {
			return BranchHistory{}, err
		}

		graph.Graph[currCommitHash] = commit
		for _, parent := range commit.Parents {
			if _, ok := graph.Graph[parent]; !ok {
				stack = append(stack, parent)
			}
		}
	}

	return graph, nil
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

// -------------------- Client Implementations -------------------- //

type Client interface {
	Init() error
	IsBranchDirty() (bool, error)
	GetCurrentBranch() (string, error)

	// Create a new branch off of the 'from' branch. If from == 'here', will get the current
	// branch and base the new branch off of it.
	CreateBranch(name, from string) error
	SwitchBranch(name string) error
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
