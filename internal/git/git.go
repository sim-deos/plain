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
	ErrNotRepo  = errors.New("not a git repository")
	ErrNotReset = errors.New("must reset")
)

type BranchHistory struct {
	Head  Commit
	Graph map[string]Commit
}

// Returns the commits display name (the first 7 characters in the commits hash)
func (c Commit) DisName() string {
	return c.Hash[:7]
}

type ObjectKind int

const (
	CommitObject ObjectKind = iota
	TreeObject
	BlobObject
	TagObject
)

type Header struct {
	Kind ObjectKind
	Size int64
}

type ObjectDecoder struct {
	compReader    io.Reader
	zlibReader    io.ReadCloser
	zlibResetter  zlib.Resetter
	headerReader  *HeaderReader
	payloadReader *bufio.Reader
}

func (d *ObjectDecoder) Reset(src io.Reader) error {
	d.compReader = src
	err := d.zlibResetter.Reset(d.compReader, nil)
	if err != nil {
		return err
	}
	d.headerReader.Reset(d.zlibReader)
	return nil
}

func (d *ObjectDecoder) Close() error {
	return d.zlibReader.Close()
}

func NewObjectDecoder(r io.Reader) (*ObjectDecoder, error) {
	compReader := r
	z, err := zlib.NewReader(compReader)
	if err != nil {
		return &ObjectDecoder{}, err
	}

	return &ObjectDecoder{
		compReader:    compReader,
		zlibReader:    z,
		zlibResetter:  z.(zlib.Resetter),
		headerReader:  NewHeaderScanner(z),
		payloadReader: bufio.NewReader(nil),
	}, nil
}

func (d *ObjectDecoder) Header() (Header, error) {
	header, payload, err := d.headerReader.Scan()
	if err != nil {
		return Header{}, err
	}
	d.payloadReader.Reset(payload)
	return header, nil
}

func (d *ObjectDecoder) DecodeCommit(hash string) (Commit, error) {
	if d.payloadReader.Buffered() == 0 {
		_, payload, err := d.headerReader.Scan()
		if err != nil {
			return Commit{}, err
		}
		d.payloadReader.Reset(payload)
	}
	return parseCommit(hash, d.payloadReader)
}

type HeaderReader struct {
	br *bufio.Reader
}

func NewHeaderScanner(r io.Reader) *HeaderReader {
	return &HeaderReader{br: bufio.NewReader(r)}
}

func (hr *HeaderReader) Reset(r io.Reader) {
	hr.br.Reset(r)
}

// Scan parses the header from a git object and returns a reader primed to read the following git objects payload.
//
// Scan() can only be called once after either initially creating the HeaderScanner or Resetting it via Reset(). Otherwise,
// an ErrNoReset err will be returned. This is the case because the HeaderScanner type is meant to read a single git object
// at a time.
func (r *HeaderReader) Scan() (Header, io.Reader, error) {
	if r.br.Buffered() > 0 {
		return Header{}, nil, ErrNotReset
	}

	kindStr, err := r.br.ReadString(' ')
	if err != nil {
		return Header{}, nil, err
	}

	var kind ObjectKind
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

	sizeStr, err := r.br.ReadString('\x00')
	if err != nil {
		return Header{}, nil, err
	}
	sizeStr = strings.TrimSuffix(sizeStr, "\x00")

	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return Header{}, nil, err
	}

	return Header{Kind: kind, Size: size}, io.LimitReader(r.br, size), nil
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

// TODO - Use sb later
func (commit Commit) String() string {
	return fmt.Sprintf(
		"Commit: %s\nTree %s\nAuthor: %s, %s, %s\nCommitter: %s, %s, %s\nMessage: %s",
		commit.Hash, commit.Tree, commit.Author.Name, commit.Author.Email, commit.Author.Time, commit.Committer.Name, commit.Committer.Email, commit.Committer.Time, commit.Message,
	)
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

// TODO - this function needs to be refactored later
func GetHistoryFor(branch string) (BranchHistory, error) {
	gitDir, err := FindGitDir()
	if err != nil {
		return BranchHistory{}, err
	}

	branchPath := filepath.Join(gitDir, "refs", "heads", branch)
	headBytes, err := os.ReadFile(branchPath)
	if err != nil {
		return BranchHistory{}, fmt.Errorf("failed to retrieve head %w", err)
	}

	headCommitStr := strings.TrimSpace(string(headBytes))
	objectsPath := filepath.Join(gitDir, "objects")
	headPath := filepath.Join(objectsPath, headCommitStr[:2], headCommitStr[2:])

	stream, _ := os.Open(headPath)
	or, err := NewObjectDecoder(stream)
	if err != nil {
		panic(err)
	}
	fmt.Println(or.Header())
	objBytes, err := os.ReadFile(headPath)
	if err != nil {
		return BranchHistory{}, fmt.Errorf("failed to read %s: %w", headPath, err)
	}

	compReader := bytes.NewReader(objBytes)
	zlibReader, err := zlib.NewReader(compReader)
	if err != nil {
		return BranchHistory{}, fmt.Errorf("failed to create decompression reader: %w", err)
	}
	defer zlibReader.Close()
	zlibResetter := zlibReader.(zlib.Resetter)
	headerScanner := NewHeaderScanner(zlibReader)
	header, payload, err := headerScanner.Scan()
	if err != nil {
		return BranchHistory{}, err
	}

	if header.Kind != CommitObject {
		return BranchHistory{}, errors.New("not a commit")
	}

	payloadReader := bufio.NewReader(payload)

	headCommit, err := parseCommit(headCommitStr, payloadReader)
	if err != nil {
		return BranchHistory{}, fmt.Errorf("failed to parse head: %w", err)
	}

	graph := BranchHistory{Head: headCommit, Graph: map[string]Commit{headCommitStr: headCommit}}
	stack := slices.Clone(headCommit.Parents)

	for len(stack) > 0 {
		currCommitHash := stack[len(stack)-1] // get last element
		stack = stack[:len(stack)-1]          // remove it (pop)

		commitPath := filepath.Join(objectsPath, currCommitHash[:2], currCommitHash[2:])

		objBytes, err = os.ReadFile(commitPath)
		if err != nil {
			return BranchHistory{}, err
		}

		// reset readers
		compReader.Reset(objBytes)
		zlibResetter.Reset(compReader, nil)
		headerScanner.Reset(zlibReader)

		header, lr, err := headerScanner.Scan()
		if err != nil {
			return BranchHistory{}, err
		}

		if header.Kind != CommitObject {
			continue
		}

		payloadReader.Reset(lr)

		commit, err := parseCommit(currCommitHash, payloadReader)
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

func parseCommit(hash string, br *bufio.Reader) (Commit, error) {
	commit := Commit{Hash: hash}

	scratch := make([]byte, 0)
	for {
		scratch = scratch[:0]

		lineBytes, moreToRead, err := br.ReadLine()
		if err != nil {
			return Commit{}, err
		}

		if moreToRead {
			scratch = append(scratch, lineBytes...)
			for moreToRead {
				moreBytes, stillMore, err := br.ReadLine()
				if err != nil {
					return Commit{}, err
				}
				scratch = append(scratch, moreBytes...)
				moreToRead = stillMore
			}
			lineBytes = scratch
		}
		if len(lineBytes) == 0 {
			break
		}
		assignCommitHeader(lineBytes, &commit)
	}

	message, err := io.ReadAll(br)
	if err != nil {
		return Commit{}, fmt.Errorf("parse: failed to parse commit message for %s. %w", hash, err)
	}
	commit.Message = strings.TrimSuffix(string(message), "\n")

	return commit, nil
}

func assignCommitHeader(line []byte, commit *Commit) error {
	sepIndex := slices.Index(line, ' ')

	if sepIndex == -1 {
		return fmt.Errorf("parse: line did not contain canonical separator")
	}

	header := string(line[:sepIndex])
	value := line[sepIndex+1:]

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
		unixTs := string(value[emailEndIndex+1:])

		colloquialTs, err := parseUnixWithOffset(string(unixTs))
		if err != nil {
			return fmt.Errorf("parse: failed due to %w", err)
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

	return nil
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
