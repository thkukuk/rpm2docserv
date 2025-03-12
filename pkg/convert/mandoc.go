package convert

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

func mandoc(r io.Reader) (stdout string, stderr string, err error) {
	stdout, stderr, err = mandocFork(r)

	// TODO(later): once a new-enough version of mandoc is in Debian,
	// get rid of this compatibility code by changing our CSS to not
	// rely on the mandoc class at all anymore.
	if err == nil && !strings.HasPrefix(stdout, `<div class="mandoc">`) {
		stdout = `<div class="mandoc">
` + stdout + `</div>
`
	}
	return stdout, stderr, err
}

// Kill mandoc after some time, it should never take more than a minute
// if it does, something is broken.
func mandocFork(r io.Reader) (stdout string, stderr string, err error) {
	var stdoutb, stderrb bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(60)*time.Second)
        defer cancel()

	cmd := exec.CommandContext(ctx, "mandoc", "-Ofragment", "-Thtml")
	cmd.Stdin = r
	cmd.Stdout = &stdoutb
	cmd.Stderr = &stderrb
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("%v, stderr: %s", err, stderrb.String())
	}
	return stdoutb.String(), stderrb.String(), nil
}
