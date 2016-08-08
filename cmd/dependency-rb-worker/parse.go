package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
)

type versionInfo struct {
	Wanted string
	Latest string
}

type logOutput struct {
	Updates        map[string]versionInfo
	LockStatements map[string]string
}

func extractUpdates(data string) map[string]versionInfo {
	var result = make(map[string]versionInfo)

	items := strings.Split(data, "\n")[1:]
	for _, item := range items {
		item = strings.TrimSpace(item)
		parts := strings.Split(item, " ")
		gem := parts[0]
		wanted := parts[3][0 : len(parts[3])-1]
		latest := parts[1][1:]
		result[gem] = versionInfo{
			Wanted: wanted,
			Latest: latest,
		}
	}

	return result
}

func ParseLog(r io.Reader) logOutput {
	bs, _ := ioutil.ReadAll(r)

	parts := strings.Split(string(bs), "\n\n")

	return logOutput{
		Updates: extractUpdates(parts[1]),
	}
}

func UpdateGemfile(deps logOutput, r io.Reader, w io.Writer) {
	bs, _ := ioutil.ReadAll(r)
	gemfile := string(bs)

	lines := strings.Split(gemfile, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}

	for name, dep := range deps.Updates {
		exp := regexp.MustCompile(
			fmt.Sprintf(
				`gem ['"]%s['"]`,
				name,
			),
		)
		for i, line := range lines {
			if exp.MatchString(line) {
				lines[i] = fmt.Sprintf(`gem '%s', '%s'`, name, dep.Latest)
				break
			}
		}
	}

	w.Write([]byte(strings.Join(lines, "\n")))
}
