package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// binaryName is the file inside the release archive.
const binaryName = "deck"

const maxBinaryBytes = 512 << 20

// checksumFor finds asset's sha256 in a checksums.txt body ("<hex>  <name>"
// per line, name optionally prefixed with "*").
func checksumFor(checksums []byte, asset string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", asset)
}

// verifyArchive checks the archive's sha256 against checksums.txt.
func verifyArchive(archive, checksums []byte, asset string) error {
	want, err := checksumFor(checksums, asset)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s: checksums.txt says %s, the download is %s", asset, want, got)
	}
	return nil
}

// extractBinary returns the deck binary from a .tar.gz archive.
func extractBinary(archive []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("unreadable archive: %w", err)
	}
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("archive has no %s binary", binaryName)
		}
		if err != nil {
			return nil, fmt.Errorf("unreadable archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && path.Base(hdr.Name) == binaryName {
			return readBinary(tr)
		}
	}
}

func readBinary(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBinaryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("unreadable archive: %w", err)
	}
	if len(data) == 0 || len(data) > maxBinaryBytes {
		return nil, fmt.Errorf("archive %s binary has an implausible size (%d bytes)", binaryName, len(data))
	}
	return data, nil
}
