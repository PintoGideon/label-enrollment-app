package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

// Processing is an administrator-owned local-development policy, not client
// input. No default credentials, cloud endpoint discovery, shell commands or
// unqualified production profiles are enabled by loading it.
type Processing struct {
	Root         string              `json:"root"`
	DockerSocket string              `json:"dockerSocket"`
	Image        string              `json:"image"`
	S3           LocalS3             `json:"s3"`
	Sources      []ProcessingSource  `json:"sources"`
	Profiles     []ProcessingProfile `json:"profiles"`
}
type LocalS3 struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}
type ProcessingSource struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"projectId"`
	Bucket    string   `json:"bucket"`
	Prefix    string   `json:"prefix"`
	Profiles  []string `json:"profiles"`
}
type ProcessingProfile struct {
	ID        string           `json:"id"`
	Purpose   string           `json:"purpose"`
	Directory string           `json:"directory"`
	Config    string           `json:"config"`
	Reference string           `json:"reference"`
	Direction string           `json:"direction"`
	Slots     []ProcessingSlot `json:"slots"`
}
type ProcessingSlot struct {
	Key        string `json:"key"`
	SummaryKey string `json:"summaryKey"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

var ErrProcessingConfig = errors.New("invalid local Workflow processing policy")

func LoadProcessing(filename string) (*Processing, error) {
	if filename == "" {
		return nil, nil
	}
	// The policy contains storage credentials. Do not include input or raw I/O
	// errors in public/logged diagnostics.
	f, e := os.Open(filename)
	if e != nil {
		return nil, ErrProcessingConfig
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() > 1<<20 {
		return nil, ErrProcessingConfig
	}
	b, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil || len(b) > 1<<20 {
		return nil, ErrProcessingConfig
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var c Processing
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || !c.valid() {
		return nil, ErrProcessingConfig
	}
	return &c, nil
}
func (c Processing) valid() bool {
	cleanAbs := func(p string) bool { return filepath.IsAbs(p) && filepath.Clean(p) == p && p != "/" }
	if !cleanAbs(c.Root) || !cleanAbs(c.DockerSocket) || !strings.HasPrefix(c.Image, "sha256:") || !api.ValidDigest(strings.TrimPrefix(c.Image, "sha256:")) {
		return false
	}
	u, e := url.Parse(c.S3.Endpoint)
	if e != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.Port() == "" {
		return false
	}
	ip, e := netip.ParseAddr(u.Hostname())
	if e != nil || !ip.IsLoopback() || ip.Zone() != "" || c.S3.Region == "" || len(c.S3.Region) > 64 || len(c.S3.AccessKey) < 3 || len(c.S3.SecretKey) < 8 {
		return false
	}
	if len(c.Profiles) < 1 || len(c.Profiles) > 32 || len(c.Sources) < 1 || len(c.Sources) > 100 {
		return false
	}
	profiles := map[string]bool{}
	for _, p := range c.Profiles {
		if !api.ValidComponent(p.ID) || profiles[p.ID] || (p.Purpose != "synthetic" && p.Purpose != "inspection") || !cleanAbs(p.Directory) || !api.ValidComponent(p.Config) || (p.Purpose == "synthetic" && !api.ValidComponent(p.Reference)) || (p.Purpose == "inspection" && p.Reference != "") || (p.Direction != "logo-first" && p.Direction != "qr-first" && p.Direction != "auto") || len(p.Slots) < 1 || len(p.Slots) > 8 {
			return false
		}
		// Neither policy assets nor output root may contain the other.
		if within(c.Root, p.Directory) || within(p.Directory, c.Root) {
			return false
		}
		profiles[p.ID] = true
		keys := map[string]bool{}
		summaries := map[string]bool{}
		for _, s := range p.Slots {
			if !api.ValidComponent(s.Key) || s.Key == "serial" || s.Key == "qr" || keys[s.Key] || !api.ValidComponent(s.SummaryKey) || summaries[s.SummaryKey] || s.Width < 1 || s.Height < 1 || s.Width > 4096 || s.Height > 4096 {
				return false
			}
			keys[s.Key] = true
			summaries[s.SummaryKey] = true
		}
	}
	sources := map[string]bool{}
	for _, s := range c.Sources {
		if !api.ValidComponent(s.ID) || sources[s.ID] || !api.ValidID(s.ProjectID) || !api.ValidComponent(s.Bucket) || len(s.Profiles) < 1 || len(s.Profiles) > 32 || len(s.Prefix) > 512 || !strings.HasSuffix(s.Prefix, "/") {
			return false
		}
		for _, part := range strings.Split(strings.TrimSuffix(s.Prefix, "/"), "/") {
			if !api.ValidComponent(part) {
				return false
			}
		}
		for _, p := range s.Profiles {
			if !profiles[p] {
				return false
			}
		}
		sources[s.ID] = true
	}
	return true
}
func within(root, p string) bool {
	rel, e := filepath.Rel(root, p)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
