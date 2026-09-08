package release

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var maxBinaryBytes int64 = 256 << 20

func Replace(ctx context.Context, c Client, rel Release, target, goos, goarch string) error {
	name := AssetName(goos, goarch)
	assetURL, ok := rel.Assets[name]
	if !ok {
		return fmt.Errorf("release %s has no %s", rel.Tag, name)
	}
	sumsURL, ok := rel.Assets["checksums.txt"]
	if !ok {
		return fmt.Errorf("release %s has no checksums.txt", rel.Tag)
	}

	want, err := digestFor(ctx, c, sumsURL, name)
	if err != nil {
		return err
	}

	dir := filepath.Dir(target)
	archive, err := stream(ctx, c, assetURL, dir, ".togen-archive-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(archive) }()

	got, err := sha256File(archive)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s does not match its checksum (%s, want %s), so %s is unchanged", name, got, want, target)
	}

	extracted, err := extract(archive, dir)
	if err != nil {
		return err
	}

	// Renaming over a running executable is safe on Linux and macOS: the
	// kernel holds the open inode, so this process finishes on the old image.
	if err := os.Rename(extracted, target); err != nil {
		_ = os.Remove(extracted)
		return fmt.Errorf("could not install over %s, which is unchanged: %w", target, err)
	}
	return nil
}

func digestFor(ctx context.Context, c Client, url, name string) (string, error) {
	body, err := c.Download(ctx, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", name)
}

func stream(ctx context.Context, c Client, url, dir, pattern string) (string, error) {
	body, err := c.Download(ctx, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	out, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	written, err := io.Copy(out, io.LimitReader(body, maxBinaryBytes+1))
	if err != nil {
		_ = out.Close()
		_ = os.Remove(out.Name())
		return "", err
	}
	if written > maxBinaryBytes {
		_ = out.Close()
		_ = os.Remove(out.Name())
		return "", fmt.Errorf("the download is larger than %d bytes", maxBinaryBytes)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func extract(archive, dir string) (string, error) {
	file, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer func() { _ = gz.Close() }()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return "", fmt.Errorf("%s has no togen entry", filepath.Base(archive))
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag != tar.TypeReg || header.Name != "togen" {
			continue
		}
		out, err := os.CreateTemp(dir, ".togen-new-*")
		if err != nil {
			return "", err
		}
		written, err := io.Copy(out, io.LimitReader(reader, maxBinaryBytes))
		if err != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
			return "", err
		}
		if written != header.Size {
			_ = out.Close()
			_ = os.Remove(out.Name())
			return "", fmt.Errorf("the togen entry is %d bytes, but only %d were read", header.Size, written)
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(out.Name())
			return "", err
		}
		if err := os.Chmod(out.Name(), 0o755); err != nil {
			_ = os.Remove(out.Name())
			return "", err
		}
		return out.Name(), nil
	}
}
