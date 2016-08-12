package repo

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

func Clone(accessToken, owner, repo string) (string, error) {
	dir, err := ioutil.TempDir("", fmt.Sprintf("gh-%s-%s", owner, repo))
	if err != nil {
		return "", err
	}

	cmds := [][]string{
		[]string{"git", "init"},
		[]string{"git", "pull", fmt.Sprintf("https://%s@github.com/%s/%s.git", accessToken, owner, repo), "--depth", "1"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}

	return dir, nil
}
