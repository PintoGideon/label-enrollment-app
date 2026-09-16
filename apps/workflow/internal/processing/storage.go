package processing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

var errInputInvalid = errors.New("input inventory or bytes do not match")

func (s *Service) objects() *s3.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DisableCompression = true
	t.ResponseHeaderTimeout = 3 * time.Second
	t.MaxResponseHeaderBytes = 16 << 10
	t.MaxConnsPerHost = 2
	h := &http.Client{Transport: t, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	c := aws.Config{Region: s.cfg.S3.Region, Credentials: credentials.NewStaticCredentialsProvider(s.cfg.S3.AccessKey, s.cfg.S3.SecretKey, ""), HTTPClient: h, RetryMaxAttempts: 1}
	return s3.NewFromConfig(c, func(o *s3.Options) { o.BaseEndpoint = aws.String(s.cfg.S3.Endpoint); o.UsePathStyle = true })
}

type listedObject struct {
	Key, ETag string
	Size      int64
}

func inventory(ctx context.Context, c *s3.Client, bucket, prefix string, expected []api.InputObject) ([]listedObject, error) {
	result := []listedObject{}
	token := ""
	tokens := map[string]bool{}
	for {
		in := &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(100)}
		if token != "" {
			in.ContinuationToken = &token
		}
		page, e := c.ListObjectsV2(ctx, in)
		if e != nil {
			return nil, storageError(e)
		}
		if len(page.CommonPrefixes) > 0 {
			return nil, errInputInvalid
		}
		for _, v := range page.Contents {
			key, etag := aws.ToString(v.Key), aws.ToString(v.ETag)
			if !strings.HasPrefix(key, prefix) || !api.ValidComponent(strings.TrimPrefix(key, prefix)) || etag == "" || len(etag) > 128 || aws.ToInt64(v.Size) < 1 || aws.ToInt64(v.Size) > api.MaxFileBytes {
				return nil, errInputInvalid
			}
			result = append(result, listedObject{key, etag, aws.ToInt64(v.Size)})
			if len(result) > len(expected) {
				return nil, errInputInvalid
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		token = aws.ToString(page.NextContinuationToken)
		if token == "" || len(token) > 4096 || tokens[token] || len(tokens) > api.MaxInputFiles {
			return nil, errInputInvalid
		}
		tokens[token] = true
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	if len(result) != len(expected) {
		return nil, errInputInvalid
	}
	for i, v := range result {
		if v.Key != prefix+expected[i].Name || v.Size != expected[i].Bytes {
			return nil, errInputInvalid
		}
	}
	return result, nil
}
func storageError(e error) error {
	var a smithy.APIError
	if errors.As(e, &a) {
		switch a.ErrorCode() {
		case "NoSuchKey", "NoSuchBucket", "NoSuchVersion", "PreconditionFailed":
			return errInputInvalid
		}
	}
	return e
}
func (s *Service) verifyInput(ctx context.Context, r api.Run) (*api.VerifiedInput, error) {
	source, ok := s.source(r.ProjectID, r.Spec)
	if !ok {
		return nil, errInputInvalid
	}
	c := s.objects()
	defer func() {
		if h, ok := c.Options().HTTPClient.(*http.Client); ok {
			h.CloseIdleConnections()
		}
	}()
	before, e := inventory(ctx, c, source.Bucket, r.Spec.Prefix, r.Spec.Objects)
	if e != nil {
		return nil, e
	}
	root := filepath.Join(s.cfg.Root, "inputs")
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	tmp, e := os.MkdirTemp(root, ".staging-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(tmp)
	input := &api.VerifiedInput{Endpoint: s.cfg.S3.Endpoint, Bucket: source.Bucket, CaptureHistory: "unknown", Objects: []api.VerifiedObject{}}
	for i, o := range before {
		response, e := c.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(source.Bucket), Key: aws.String(o.Key), IfMatch: aws.String(o.ETag)})
		if e != nil {
			return nil, storageError(e)
		}
		b, e := io.ReadAll(io.LimitReader(response.Body, r.Spec.Objects[i].Bytes+1))
		_ = response.Body.Close()
		if e != nil {
			return nil, e
		}
		expected := r.Spec.Objects[i]
		if aws.ToInt64(response.ContentLength) != expected.Bytes || aws.ToString(response.ETag) != o.ETag || int64(len(b)) != expected.Bytes || api.Hash(b) != expected.SHA256 {
			return nil, errInputInvalid
		}
		dim, kind, e := image.DecodeConfig(bytes.NewReader(b))
		if e != nil || (kind != "png" && kind != "jpeg") || dim.Width < 1 || dim.Height < 1 || dim.Width > 8192 || dim.Height > 8192 {
			return nil, errInputInvalid
		}
		version := aws.ToString(response.VersionId)
		if len(version) > 1024 {
			return nil, errInputInvalid
		}
		input.Objects = append(input.Objects, api.VerifiedObject{InputObject: expected, Key: o.Key, VersionID: version, ETag: o.ETag})
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		f, e := os.OpenFile(filepath.Join(tmp, expected.Name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
		if e != nil {
			return nil, e
		}
		_, e = f.Write(b)
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	after, e := inventory(ctx, c, source.Bucket, r.Spec.Prefix, r.Spec.Objects)
	if e != nil {
		return nil, e
	}
	if !reflect.DeepEqual(before, after) {
		return nil, errInputInvalid
	}
	input.Digest = input.SealedDigest()
	snapshot := filepath.Join(root, input.Digest)
	if _, e = os.Lstat(snapshot); e == nil {
		check := r
		check.Input = input
		if e = s.checkInput(ctx, check); e != nil {
			return nil, e
		}
	} else if errors.Is(e, os.ErrNotExist) {
		if e = os.Rename(tmp, snapshot); e != nil {
			return nil, e
		}
		if dir, e := os.Open(root); e == nil {
			syncErr := dir.Sync()
			_ = dir.Close()
			if syncErr != nil {
				return nil, syncErr
			}
		} else {
			return nil, e
		}
	} else {
		return nil, e
	}
	return input, nil
}
func (s *Service) inputDir(r api.Run) string {
	return filepath.Join(s.cfg.Root, "inputs", r.Input.Digest)
}
func (s *Service) checkInput(ctx context.Context, r api.Run) error {
	if r.Input == nil || r.Input.Digest != r.Input.SealedDigest() || len(r.Input.Objects) != len(r.Spec.Objects) {
		return errInputInvalid
	}
	root, e := os.OpenRoot(s.inputDir(r))
	if e != nil {
		return errInputInvalid
	}
	defer root.Close()
	dir, e := root.Open(".")
	if e != nil {
		return errInputInvalid
	}
	entries, e := dir.ReadDir(api.MaxInputFiles + 1)
	_ = dir.Close()
	if e != nil && e != io.EOF {
		return errInputInvalid
	}
	if len(entries) != len(r.Spec.Objects) {
		return errInputInvalid
	}
	for i, o := range r.Spec.Objects {
		if e = ctx.Err(); e != nil {
			return e
		}
		if r.Input.Objects[i].InputObject != o {
			return errInputInvalid
		}
		st, e := root.Lstat(o.Name)
		if e != nil || !st.Mode().IsRegular() || st.Size() != o.Bytes {
			return errInputInvalid
		}
		f, e := root.Open(o.Name)
		if e != nil {
			return errInputInvalid
		}
		hash := sha256.New()
		n, e := io.Copy(hash, io.LimitReader(f, o.Bytes+1))
		_ = f.Close()
		if e != nil || n != o.Bytes || hex.EncodeToString(hash.Sum(nil)) != o.SHA256 {
			return errInputInvalid
		}
	}
	return nil
}
