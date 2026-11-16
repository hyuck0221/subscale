package update

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
)

func extract(reader io.Reader, dest io.Writer) error {
	gz, e := gzip.NewReader(reader)
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(io.LimitReader(gz, maxArchive+1))
	found := false
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if h.Name != "subscale" {
			continue
		}
		if found || h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > maxArchive {
			return fmt.Errorf("invalid executable in release archive")
		}
		found = true
		if _, e = io.Copy(dest, tr); e != nil {
			return e
		}
	}
	if !found {
		return fmt.Errorf("release archive contains no subscale executable")
	}
	return nil
}
