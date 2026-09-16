// Package processing coordinates the existing image processor. It contains no
// image matching/stitching algorithm and makes no APID calls.
package processing

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/config"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

type profile struct {
	Policy                       config.ProcessingProfile
	Digest, ReferenceDigest, Dir string
	References                   map[string]string
}

func readFile(root, name string, limit int64) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, errors.New("invalid artifact path")
	}
	r, e := os.OpenRoot(root)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	st, e := r.Lstat(name)
	if e != nil || !st.Mode().IsRegular() {
		return nil, errors.New("artifact is not a regular file")
	}
	f, e := r.Open(name)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e = f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() > limit {
		return nil, errors.New("artifact exceeds bound")
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, errors.New("artifact is unreadable")
	}
	return b, nil
}
func bundle(dir string) (map[string][]byte, []api.InputObject, error) {
	files := map[string][]byte{}
	list := []api.InputObject{}
	var total int64
	e := filepath.WalkDir(dir, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink in asset bundle")
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(dir, p)
		if e != nil {
			return e
		}
		name := filepath.ToSlash(rel)
		for _, s := range strings.Split(name, "/") {
			if !api.ValidComponent(s) {
				return errors.New("invalid asset name")
			}
		}
		b, e := readFile(dir, name, api.MaxFileBytes)
		if e != nil {
			return e
		}
		total += int64(len(b))
		if total > 256<<20 || len(list) >= 1000 {
			return errors.New("asset bundle exceeds bounds")
		}
		files[name] = b
		list = append(list, api.InputObject{Name: name, Bytes: int64(len(b)), SHA256: api.Hash(b)})
		return nil
	})
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return files, list, e
}
func publishDir(root, target string, files map[string][]byte) error {
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	// Never rewrite an existing snapshot. Verify byte identity on crash recovery.
	if _, e := os.Lstat(filepath.Join(root, target)); e == nil {
		existing, _, e := bundle(filepath.Join(root, target))
		if e != nil || len(existing) != len(files) {
			return errors.New("snapshot differs")
		}
		for n, b := range files {
			if !bytes.Equal(existing[n], b) {
				return errors.New("snapshot differs")
			}
		}
		return nil
	}
	tmp, e := os.MkdirTemp(root, ".staging-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	for n, b := range files {
		if !fs.ValidPath(n) {
			return errors.New("invalid snapshot path")
		}
		p := filepath.Join(tmp, filepath.FromSlash(n))
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		if e = os.WriteFile(p, b, 0400); e != nil {
			return e
		}
	}
	return os.Rename(tmp, filepath.Join(root, target))
}
func loadProfile(root string, p config.ProcessingProfile) (profile, error) {
	files, inventory, e := bundle(p.Directory)
	if e != nil {
		return profile{}, e
	}
	if len(files[p.Config]) == 0 || (p.Reference != "" && len(files[p.Reference]) == 0) {
		return profile{}, errors.New("missing profile assets")
	}
	var rows [][]string
	referenceDigest := ""
	if p.Reference != "" {
		rows, e = csv.NewReader(bytes.NewReader(files[p.Reference])).ReadAll()
		if e != nil || len(rows) < 1 || len(rows) > api.MaxLabels {
			return profile{}, errors.New("invalid serial authority")
		}
		referenceDigest = api.Hash(files[p.Reference])
	}
	refs := map[string]string{}
	qrSeen := map[string]bool{}
	for _, row := range rows {
		if len(row) != 2 || !api.ValidComponent(row[0]) || len(row[1]) < 1 || len(row[1]) > 2048 || refs[row[0]] != "" || qrSeen[row[1]] {
			return profile{}, errors.New("ambiguous serial authority")
		}
		refs[row[0]] = row[1]
		qrSeen[row[1]] = true
	}
	canonical := p
	canonical.Directory = ""
	digest := api.Digest(struct {
		Policy config.ProcessingProfile
		Files  []api.InputObject
	}{canonical, inventory})
	dir := filepath.Join(root, "profiles")
	if e = publishDir(dir, digest, files); e != nil {
		return profile{}, e
	}
	return profile{Policy: p, Digest: digest, ReferenceDigest: referenceDigest, Dir: filepath.Join(dir, digest), References: refs}, nil
}
