package git

import (
	"fmt"
	"os"
	"os/exec"
)

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
