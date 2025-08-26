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
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var (
	ErrNotRepo       = errors.New("not a git repository")
	ErrNotReset      = errors.New("must reset")
	ErrUnknownObject = errors.New("unknown object kind")
)

// GitObjectKind represents the type of object stored in a Git repository.
//
// Git defines four fundamental object kinds: commit, tree, blob, and tag.
type GitObjectKind int

const (
	// CommitObject identifies a Git commit object,
	// which records a snapshot of the repository state.
	CommitObject GitObjectKind = iota + 1

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

// Pre-allocated slices to reduce string allocation in object parsing
var (
	bCommit    = []byte("commit")
	bBlob      = []byte("blob")
	bTag       = []byte("tag")
	bParent    = []byte("parent")
	bTree      = []byte("tree")
	bAuthor    = []byte("author")
	bCommitter = []byte("committer")
)

var gitObjectName = map[GitObjectKind]string{
	CommitObject: "commit",
	TreeObject:   "tree",
	BlobObject:   "blob",
	TagObject:    "tag",
}

func (obj GitObjectKind) String() string {
	return gitObjectName[obj]
}

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

// Reads the [Header] of the current git object.
// A header contains the objects kind and size in bytes.
//
// If the git object itself was already decoded, than this method will fail unless the Decoder is reset.
func (d *Decoder) Header() (ObjectHeader, error) {
	// Check if data has already been read (indicating reset is needed)
	if d.br.Buffered() > 0 {
		return ObjectHeader{}, ErrNotReset
	}

	line, err := d.br.ReadSlice('\x00')
	if err != nil {
		return ObjectHeader{}, err
	}
	line = line[:len(line)-1]

	var kind GitObjectKind
	switch {
	case bytes.HasPrefix(line, bCommit):
		kind = CommitObject
	case bytes.HasPrefix(line, bTree):
		kind = TreeObject
	case bytes.HasPrefix(line, bBlob):
		kind = BlobObject
	case bytes.HasPrefix(line, bTag):
		kind = TagObject
	default:
		return ObjectHeader{}, fmt.Errorf("%w: %q", ErrUnknownObject, line)
	}

	// commit 262
	sepIndex := slices.Index(line, ' ')

	size, err := bytesToInt64(line[sepIndex+1:])
	if err != nil {
		return ObjectHeader{}, err
	}

	return ObjectHeader{Kind: kind, Size: size}, nil
}

// Reads, decodes, and returns the current git object.
// Once this method is called, Header() will result in an error.
func (d *Decoder) DecodeCommit(hash string) (Commit, error) {
	commit := Commit{Hash: hash}
	for {
		lineBytes, err := d.br.ReadSlice('\n')
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
			return Commit{}, fmt.Errorf("parse: line did not contain canonical separator: %s", lineBytes)
		}

		header, value := lineBytes[:sepIndex], lineBytes[sepIndex+1:]
		switch {
		case bytes.Equal(header, bTree):
			commit.Tree = string(value)
		case bytes.Equal(header, bParent):
			commit.Parents = append(commit.Parents, string(value))
		case bytes.Equal(header, bAuthor), bytes.Equal(header, bCommitter):
			emailStartIndex := slices.Index(value, '<')
			emailEndIndex := slices.Index(value, '>')

			email := string(value[emailStartIndex+1 : emailEndIndex])
			name := string(value[:emailStartIndex-1])
			timestamp, err := parseGitUnixTs(value[emailEndIndex+2:])
			if err != nil {
				return Commit{}, fmt.Errorf("parse: failed to parse commit due to time error %w", err)
			}

			sig := Signature{
				Name:  name,
				Email: email,
				Time:  timestamp,
			}

			if bytes.HasPrefix(header, bAuthor) {
				commit.Author = sig
			} else {
				commit.Committer = sig
			}
		}
	}

	message, err := io.ReadAll(d.br)
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

func parseGitUnixTs(timestamp []byte) (time.Time, error) {
	sepIndex := slices.Index(timestamp, ' ')

	var sign int
	if timestamp[sepIndex+1] == '-' {
		sign = -1
	} else {
		sign = 1
	}

	sec, err := bytesToInt64(timestamp[:sepIndex])
	if err != nil {
		return time.Time{}, err
	}
	hours, err := bytesToInt(timestamp[sepIndex+2 : sepIndex+4])
	if err != nil {
		return time.Time{}, err
	}
	mins, err := bytesToInt(timestamp[sepIndex+4:])
	if err != nil {
		return time.Time{}, err
	}
	offset := sign * (hours*3600 + mins*60)
	loc := time.FixedZone(string(timestamp[sepIndex+1:]), offset)
	return time.Unix(sec, 0).In(loc), nil
}

func bytesToInt64(n []byte) (int64, error) {
	var i int64
	for _, c := range n {
		if c < '0' || c > '9' {
			return -1, fmt.Errorf("unrecognized value in git timestamp (%c) in: %s", c, n)
		}
		i = i*10 + int64(c-'0')
	}
	return i, nil
}

func bytesToInt(n []byte) (int, error) {
	var i int
	for _, c := range n {
		if c < '0' || c > '9' {
			return -1, fmt.Errorf("unrecognized value in git timestamp: %c", c)
		}
		i = i*10 + int(c-'0')
	}
	return i, nil
}
