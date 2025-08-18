/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sim-deos/plain/internal/git"
)

var (
	treeLine     = []byte("tree ")
	parentLine   = []byte("parent ")
	authorLine   = []byte("author ")
	commiterLine = []byte("committer ")
)

type GitObject struct {
	Type    string
	Size    int64
	Content []byte
}

type CommitNode struct {
	val     Commit
	parents []CommitNode
}

func (node *CommitNode) Add(parent CommitNode) {
	node.parents = append(node.parents, parent)
}

func (node *CommitNode) Items() []CommitNode {
	return node.parents
}

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

func main() {
	git.GetHistoryFor("main")
}

func historyForBranch(branch string) {
	initialCommit, err := os.ReadFile("./.git/refs/heads/" + branch)
	if err != nil {
		fmt.Println("ran into an error:", err.Error())
	}

	commitHash := string(initialCommit[:len(initialCommit)-1])

	head, err := buildTree(commitHash)
	if err != nil {
		fmt.Println("got error:", err.Error())
	}

	fmt.Println("Head = ", len(head.parents))

	printTree(head)
}

func printTree(head *CommitNode) {
	if head.val.Hash == "" {
		return
	}

	fmt.Println(head.val.Hash)
	for _, parent := range head.parents {
		printTree(&parent)
	}
}

func buildTree(commit string) (*CommitNode, error) {
	if commit == "" {
		return &CommitNode{}, fmt.Errorf("cannot build out graph for non-existent commits")
	}

	fb, err := getCommitFileBytes(commit)
	if err != nil {
		return &CommitNode{}, fmt.Errorf("failed to get initial commit file %w", err)
	}

	commitObj, err := createCommit(commit, bytes.NewReader(fb))
	if err != nil {
		return &CommitNode{}, err
	}

	return commitObj, nil
}

func getCommitFileBytes(commit string) ([]byte, error) {
	commit, _ = strings.CutSuffix(commit, "\n")

	dirName := commit[:2]
	fileName := commit[2:]

	fb, err := os.ReadFile("./.git/objects/" + dirName + "/" + fileName)
	if err != nil {
		return nil, err
	}

	return fb, nil
}

func createCommit(hash string, cr io.Reader) (*CommitNode, error) {
	zlibR, err := zlib.NewReader(cr)
	if err != nil {
		return &CommitNode{}, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	defer zlibR.Close()

	br := bufio.NewReader(zlibR)

	head := &CommitNode{
		val: Commit{Hash: hash},
	}
	for {
		bs, _ := io.ReadAll(zlibR)
		fmt.Println(string(bs))
		line, err := br.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return &CommitNode{}, err
		} else if err != nil && err == io.EOF {
			return head, nil
		}

		switch {
		case bytes.HasPrefix(line, treeLine):
			head.val.Tree = string(line[5:])
		case bytes.HasPrefix(line, parentLine):
			parentHash := strings.TrimSpace(string(line[7:]))
			parent, err := buildTree(parentHash)
			fmt.Printf("Found parent: %s\n", parent.val.Hash)
			if err != nil {
				return &CommitNode{}, fmt.Errorf("error occurred while building out tree: %w", err)
			}
		case bytes.HasPrefix(line, authorLine):
			//fmt.Println("Not bothering with author right now")
		case bytes.HasPrefix(line, commiterLine):
			//fmt.Println("Not bothering with committer right now")
		}
	}
}
