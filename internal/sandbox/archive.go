package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"

	"github.com/moby/moby/client"
)

// A file crosses into a container as a tar archive: the Engine API has no
// endpoint that takes a plain file.

// ReadFile returns the content of one file in the container.
func (c *Container) ReadFile(ctx context.Context, name string) (string, error) {
	copied, err := c.docker.CopyFromContainer(ctx, c.id, client.CopyFromContainerOptions{SourcePath: c.resolve(name)})
	if err != nil {
		return "", err
	}
	defer copied.Content.Close()

	reader := tar.NewReader(copied.Content)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return "", fmt.Errorf("%s is empty", name)
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		content, err := io.ReadAll(reader)
		return string(content), err
	}
}

// WriteFile replaces the content of one file in the container and creates the
// directories above it.
func (c *Container) WriteFile(ctx context.Context, name, content string) error {
	full := c.resolve(name)
	parent := path.Dir(full)

	// The archive endpoint extracts into a directory that must already exist.
	if _, err := c.Run(ctx, "mkdir -p "+shellQuote(parent)); err != nil {
		return err
	}

	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	header := &tar.Header{
		Name:     path.Base(full),
		Mode:     0o644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, content); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	_, err := c.docker.CopyToContainer(ctx, c.id, client.CopyToContainerOptions{DestinationPath: parent, Content: &archive})
	return err
}
