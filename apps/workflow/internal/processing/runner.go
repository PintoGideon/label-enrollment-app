package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

var errRunner = errors.New("local execution requires reconciliation")

type dockerState struct {
	ID     string `json:"Id"`
	Name   string
	Config struct {
		Image  string
		Labels map[string]string
	}
	State struct {
		Status    string
		Running   bool
		ExitCode  int
		OOMKilled bool
		Error     string
	}
}

func (s *Service) docker(ctx context.Context, method, path string, payload any) ([]byte, int, error) {
	transport := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 3 * time.Second, MaxResponseHeaderBytes: 16 << 10, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", s.cfg.DockerSocket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var b []byte
	if payload != nil {
		b, _ = json.Marshal(payload)
	}
	req, e := http.NewRequestWithContext(ctx, method, "http://docker/v1.45"+path, bytes.NewReader(b))
	if e != nil {
		return nil, 0, errRunner
	}
	req.Header.Set("Content-Type", "application/json")
	response, e := client.Do(req)
	if e != nil {
		return nil, 0, errRunner
	}
	defer response.Body.Close()
	b, e = io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if e != nil || len(b) > 256<<10 {
		return nil, 0, errRunner
	}
	return b, response.StatusCode, nil
}
func attemptName(r api.Run) string { return "labeltron-" + r.Attempt.ID }
func (s *Service) attemptDir(r api.Run) string {
	return filepath.Join(s.cfg.Root, "attempts", r.Attempt.ID)
}
func (s *Service) executionDigest(r api.Run) string {
	return api.Digest(struct{ Run, Attempt, Input, Profile, Image, Root string }{r.ID, r.Attempt.ID, r.Input.Digest, r.ProfileDigest, r.Attempt.Image, s.cfg.Root})
}
func (s *Service) inspect(ctx context.Context, r api.Run) (*dockerState, error) {
	b, status, e := s.docker(ctx, "GET", "/containers/"+attemptName(r)+"/json", nil)
	if e != nil {
		return nil, e
	}
	if status == 404 {
		return nil, nil
	}
	if status != 200 {
		return nil, errRunner
	}
	var v dockerState
	if json.Unmarshal(b, &v) != nil || v.ID == "" || v.Name != "/"+attemptName(r) || v.Config.Image != r.Attempt.Image || v.Config.Labels["labeltron.attempt"] != r.Attempt.ID || v.Config.Labels["labeltron.execution"] != s.executionDigest(r) {
		return nil, errRunner
	}
	return &v, nil
}

// The fixed launch guard complements database fencing. Docker start is not a
// compare-and-swap: an old, delayed start could restart an exited container.
// mkdir is exclusive on the attempt volume, so that can NEVER execute the
// algorithm twice. Ambiguous/guard-failed attempts require cancellation and a
// fresh attempt; existing output is not sufficient evidence of success.
const launchGuard = `mkdir /output/.execution-once || exit 79
if test -e /output/.cancelled; then exit 78; fi
exec /usr/local/bin/stitchin-complete "$@"`

func (s *Service) createExecution(ctx context.Context, r api.Run, p profile) error {
	if e := os.MkdirAll(s.attemptDir(r), 0700); e != nil {
		return e
	}
	bind := func(source, target string, readonly bool) map[string]any {
		return map[string]any{"Type": "bind", "Source": source, "Target": target, "ReadOnly": readonly}
	}
	spec := map[string]any{
		"Image": r.Attempt.Image, "User": strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), "WorkingDir": "/work",
		"Entrypoint": []string{"/bin/sh", "-c", launchGuard, "stitcher"},
		"Cmd":        []string{p.Policy.Config, "--source", "/input", "--name", r.Attempt.ID, "--out-root", "/output", "--jobs", "4", "--direction", p.Policy.Direction},
		"Labels":     map[string]string{"labeltron.attempt": r.Attempt.ID, "labeltron.execution": s.executionDigest(r)},
		"HostConfig": map[string]any{"NetworkMode": "none", "ReadonlyRootfs": true, "Memory": int64(4 << 30), "NanoCpus": int64(4e9), "PidsLimit": 128, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges"}, "RestartPolicy": map[string]string{"Name": "no"}, "LogConfig": map[string]any{"Type": "json-file", "Config": map[string]string{"max-size": "8m", "max-file": "1"}}, "Tmpfs": map[string]string{"/tmp": "rw,noexec,nosuid,size=16777216"}, "Mounts": []any{bind(p.Dir, "/work", true), bind(s.inputDir(r), "/input", true), bind(s.attemptDir(r), "/output", false)}},
	}
	_, status, e := s.docker(ctx, "POST", "/containers/create?name="+attemptName(r), spec)
	if e != nil {
		return e
	}
	if status != 201 && status != 409 {
		return errRunner
	}
	return nil
}
func (s *Service) cancelExecution(ctx context.Context, r api.Run) (bool, error) {
	// Persist this before observing "not found". Any delayed create/start can
	// subsequently execute only the guard, not the image-processing algorithm.
	if e := os.MkdirAll(s.attemptDir(r), 0700); e != nil {
		return false, e
	}
	f, e := os.OpenFile(filepath.Join(s.attemptDir(r), ".cancelled"), os.O_WRONLY|os.O_CREATE, 0600)
	if e != nil {
		return false, e
	}
	e = f.Sync()
	_ = f.Close()
	if e != nil {
		return false, e
	}
	v, e := s.inspect(ctx, r)
	if e != nil {
		return false, e
	}
	if v == nil || v.State.Status == "created" || v.State.Status == "exited" || v.State.Status == "dead" {
		return true, nil
	}
	if !v.State.Running {
		return false, errRunner
	}
	_, status, e := s.docker(ctx, "POST", "/containers/"+attemptName(r)+"/kill?signal=SIGKILL", nil)
	if e != nil || status != 204 {
		return false, errRunner
	}
	v, e = s.inspect(ctx, r)
	return e == nil && v != nil && !v.State.Running && (v.State.Status == "exited" || v.State.Status == "dead"), e
}
func (s *Service) executionStep(ctx context.Context, r *api.Run) {
	if r.Attempt == nil || r.Input == nil {
		r.State = "failed"
		r.FailureCode = "INPUT_INVALID"
		return
	}
	if r.State == "cancelling" {
		done, e := s.cancelExecution(ctx, *r)
		if e == nil && done {
			r.State = "cancelled"
			r.Progress.Stage = "cancelled"
		}
		return
	}
	if time.Since(r.Attempt.CreatedAt) > time.Hour {
		r.State = "cancelling"
		r.FailureCode = "EXECUTION_TIMEOUT"
		return
	}
	p, ok := s.profiles[r.Spec.ProfileID]
	if !ok || p.Digest != r.ProfileDigest || s.cfg.Image != r.Attempt.Image {
		r.State = "cancelling"
		r.FailureCode = "POLICY_CHANGED"
		return
	}
	if _, ok = s.source(r.ProjectID, r.Spec); !ok {
		r.State = "cancelling"
		r.FailureCode = "POLICY_CHANGED"
		return
	}
	v, e := s.inspect(ctx, *r)
	if e != nil {
		r.State = "uncertain"
		r.FailureCode = "RUNNER_UNCERTAIN"
		return
	}
	if v == nil {
		if r.State == "running" || r.State == "validating" {
			r.State = "uncertain"
			r.FailureCode = "RUNNER_UNCERTAIN"
			return
		}
		// A missing container after a possible earlier execution is not permission
		// to recreate it. The durable launch marker is the local safety backstop.
		if _, e = os.Lstat(filepath.Join(s.attemptDir(*r), ".execution-once")); e == nil {
			r.State = "uncertain"
			r.FailureCode = "RUNNER_UNCERTAIN"
			return
		}
		if e = s.checkInput(ctx, *r); e != nil {
			r.State = "failed"
			r.FailureCode = "INPUT_INVALID"
			return
		}
		if e = s.checkProfile(p); e != nil {
			r.State = "failed"
			r.FailureCode = "PROFILE_CHANGED"
			return
		}
		if e = s.createExecution(ctx, *r, p); e != nil {
			r.State = "uncertain"
			r.FailureCode = "RUNNER_UNCERTAIN"
			return
		}
		v, e = s.inspect(ctx, *r)
		if e != nil || v == nil {
			r.State = "uncertain"
			r.FailureCode = "RUNNER_UNCERTAIN"
			return
		}
	}
	switch v.State.Status {
	case "created":
		if e = s.checkInput(ctx, *r); e != nil {
			r.State = "cancelling"
			r.FailureCode = "INPUT_INVALID"
			return
		}
		if e = s.checkProfile(p); e != nil {
			r.State = "cancelling"
			r.FailureCode = "PROFILE_CHANGED"
			return
		}
		_, status, e := s.docker(ctx, "POST", "/containers/"+attemptName(*r)+"/start", nil)
		if e != nil || (status != 204 && status != 304) {
			r.State = "uncertain"
			r.FailureCode = "RUNNER_UNCERTAIN"
			return
		}
		r.State = "running"
		r.FailureCode = ""
		r.Progress.Stage = "running"
	case "running":
		r.State = "running"
		r.FailureCode = ""
		if progress, ok := s.engineProgress(*r); ok {
			r.Progress = progress
		}
	case "exited":
		exit := v.State.ExitCode
		r.Attempt.ExitCode = &exit
		if exit == 79 {
			r.State = "uncertain"
			r.FailureCode = "RUNNER_UNCERTAIN"
			return
		}
		if exit == 78 {
			r.State = "cancelled"
			r.Progress.Stage = "cancelled"
			return
		}
		if exit != 0 || v.State.OOMKilled || v.State.Error != "" {
			r.State = "failed"
			r.FailureCode = "PROCESS_FAILED"
			return
		}
		r.State = "validating"
		r.FailureCode = ""
		if progress, ok := s.engineProgress(*r); ok {
			r.Progress = progress
		}
	default:
		r.State = "uncertain"
		r.FailureCode = "RUNNER_UNCERTAIN"
	}
}
func (s *Service) checkProfile(p profile) error {
	_, inventory, e := bundle(p.Dir)
	if e != nil {
		return e
	}
	policy := p.Policy
	policy.Directory = ""
	if api.Digest(struct {
		Policy any
		Files  []api.InputObject
	}{policy, inventory}) != p.Digest {
		return errors.New("profile snapshot changed")
	}
	return nil
}

type performance struct {
	Schema   int    `json:"schema_version"`
	Run      string `json:"run"`
	Finished string `json:"finished"`
	Frames   int    `json:"frames"`
	Phases   []struct {
		Phase string `json:"phase"`
	} `json:"phases"`
	Extraction struct {
		Boundaries int `json:"boundaries"`
	} `json:"extraction"`
	Regroup struct {
		Labels   int `json:"labels"`
		Complete int `json:"labels_complete"`
		Crops    int `json:"crops_rendered"`
		Failed   int `json:"crops_failed"`
	} `json:"regroup"`
}

func (s *Service) outputDir(r api.Run) string { return filepath.Join(s.attemptDir(r), r.Attempt.ID) }
func (s *Service) readPerformance(r api.Run) (performance, error) {
	var v performance
	b, e := readFile(s.outputDir(r), "performance.json", 64<<10)
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil || v.Schema != 1 || v.Run != r.Attempt.ID || v.Frames != len(r.Spec.Objects) || v.Extraction.Boundaries < 0 || v.Extraction.Boundaries > api.MaxInputFiles+2 || v.Regroup.Labels < 0 || v.Regroup.Labels > api.MaxInputFiles {
		return v, errors.New("invalid progress")
	}
	return v, nil
}
func (s *Service) engineProgress(r api.Run) (api.Progress, bool) {
	v, e := s.readPerformance(r)
	if e != nil {
		return api.Progress{}, false
	}
	stage := "running"
	for _, phase := range v.Phases {
		switch phase.Phase {
		case "matching":
			stage = "matching_complete"
		case "extraction":
			stage = "extraction_complete"
		case "regroup (incl. crops)":
			stage = "regroup_complete"
		case "total":
			stage = "processing_complete"
		}
	}
	return api.Progress{Stage: stage, Frames: v.Frames, Boundaries: v.Extraction.Boundaries, Labels: v.Regroup.Labels, Crops: v.Regroup.Crops}, true
}
