package fort

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/render"
)

const (
	Container    = "fort"
	Product      = "fort"
	Image        = "userland/fort:0.19.1"
	Host         = "fort"
	Files        = "FORT_FILES"
	Separator    = ":"
	Schedule     = "FORT_SCHEDULE"
	Bucket       = "FORT_S3"
	Store        = "FORT_KEY"
	FilesTag     = "files"
	keyCommand   = "/usr/local/bin/fort-key"
	noRepository = 10
	wrongKey     = 12
)

type State string

const (
	Absent  State = "absent"
	Ready   State = "ready"
	Foreign State = "foreign"
)

type Node struct {
	Type     string    `json:"type"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Mode     uint32    `json:"mode"`
	Modified time.Time `json:"mtime"`
}

func BucketVariables() []string {
	return []string{
		Bucket + "_" + manifest.Name,
		Bucket + "_" + manifest.Region,
		Bucket + "_" + manifest.Endpoint,
		Bucket + "_" + manifest.AccessKeyID,
		Bucket + "_" + manifest.SecretAccessKey,
	}
}

func StoreVariables() []string {
	return []string{
		Store + "_" + manifest.Provider,
		Store + "_" + manifest.Parameter,
		Store + "_" + manifest.Region,
		Store + "_" + manifest.AccessKeyID,
		Store + "_" + manifest.SecretAccessKey,
	}
}

func Environment(e *env.File) ([]string, error) {
	needed := append(BucketVariables(), StoreVariables()...)
	if err := e.Require(needed...); err != nil {
		return nil, fmt.Errorf("fort cannot reach its bucket or its master key: %w", err)
	}
	endpoint := strings.TrimSuffix(e.Get(Bucket+"_"+manifest.Endpoint), "/")
	lines := []string{
		"RESTIC_REPOSITORY=s3:" + endpoint + "/" + e.Get(Bucket+"_"+manifest.Name),
		"AWS_ACCESS_KEY_ID=" + e.Get(Bucket+"_"+manifest.AccessKeyID),
		"AWS_SECRET_ACCESS_KEY=" + e.Get(Bucket+"_"+manifest.SecretAccessKey),
		"AWS_DEFAULT_REGION=" + e.Get(Bucket+"_"+manifest.Region),
		"RESTIC_PASSWORD_COMMAND=" + keyCommand,
		"RESTIC_HOST=" + Host,
	}
	for _, name := range StoreVariables() {
		lines = append(lines, name+"="+e.Get(name))
	}
	if url := os.Getenv("AWS_ENDPOINT_URL"); url != "" {
		lines = append(lines, "AWS_ENDPOINT_URL="+url)
	}
	return lines, nil
}

func Build(root string) error {
	cmd := exec.Command("docker", "build", "-q", "-t", Image, "-f", filepath.Join(root, "Dockerfile"), root)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("building %s: %s", Image, strings.TrimSpace(string(out)))
	}
	return nil
}

func Probe(e *env.File) (State, error) {
	_, err := Output(e, nil, "cat", "config", "--no-lock")
	if err == nil {
		return Ready, nil
	}
	switch status(err) {
	case noRepository:
		return Absent, nil
	case wrongKey:
		return Foreign, nil
	}
	return "", err
}

func Init(e *env.File) (string, error) {
	return Output(e, nil, "init")
}

func Kept(e *env.File) []string {
	return e.ListBy(Files, Separator)
}

func Keep(e *env.File, paths []string) {
	e.Set(Files, strings.Join(paths, Separator))
}

func Backup() error {
	cmd := exec.Command("docker", "exec", Container, "/fort-entrypoint.sh", "backup")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func Listing(e *env.File) ([]Node, error) {
	out, err := Output(e, nil, "ls", "--json", "--no-lock", "--recursive", "--tag", FilesTag, "latest", render.FilesRoot)
	if err != nil {
		return nil, err
	}
	var nodes []Node
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var n Node
		if err := json.Unmarshal([]byte(line), &n); err != nil || n.Type != "file" {
			continue
		}
		n.Path = strings.TrimPrefix(n.Path, render.FilesRoot)
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("the latest snapshot holds no file under %s", render.FilesRoot)
	}
	return nodes, nil
}

func Extract(e *env.File, target string) ([]string, error) {
	lines, err := Environment(e)
	if err != nil {
		return nil, err
	}
	file, remove, err := envFile(lines)
	if err != nil {
		return nil, err
	}
	defer remove()
	cmd := exec.Command("docker", "run", "--rm", "--env-file", file, Image,
		"dump", "--no-lock", "--tag", FilesTag, "latest:"+render.FilesRoot, "/", "--archive", "tar")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	written, extractErr := untar(stdout, target)
	io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil {
		return written, fmt.Errorf("restic dump: %s", strings.TrimSpace(stderr.String()))
	}
	return written, extractErr
}

func untar(stream io.Reader, target string) ([]string, error) {
	var written []string
	reader := tar.NewReader(stream)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return written, nil
		}
		if err != nil {
			return written, err
		}
		path := filepath.Join(target, filepath.Clean("/"+header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, header.FileInfo().Mode().Perm()); err != nil {
				return written, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return written, err
			}
			out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, header.FileInfo().Mode().Perm())
			if err != nil {
				return written, err
			}
			if _, err := io.Copy(out, reader); err != nil {
				out.Close()
				return written, err
			}
			if err := out.Close(); err != nil {
				return written, err
			}
			os.Chtimes(path, header.ModTime, header.ModTime)
			written = append(written, path)
		}
	}
}

func Output(e *env.File, mounts []string, args ...string) (string, error) {
	lines, err := Environment(e)
	if err != nil {
		return "", err
	}
	file, remove, err := envFile(lines)
	if err != nil {
		return "", err
	}
	defer remove()
	argv := []string{"run", "--rm", "--env-file", file}
	for _, mount := range mounts {
		argv = append(argv, "-v", mount)
	}
	argv = append(argv, Image)
	out, err := exec.Command("docker", append(argv, args...)...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

func envFile(lines []string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "userland-fort-")
	if err != nil {
		return "", nil, err
	}
	remove := func() { os.RemoveAll(dir) }
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		remove()
		return "", nil, err
	}
	return path, remove, nil
}

func status(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}
