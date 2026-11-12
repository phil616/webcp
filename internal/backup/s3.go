package backup

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"web-panel/internal/domain"
)

type S3 struct {
	client *minio.Client
	config domain.S3
}

func NewS3(config domain.S3) (*S3, error) {
	if err := domain.ValidateS3(config); err != nil {
		return nil, err
	}
	if config.Endpoint == "" {
		return nil, fmt.Errorf("尚未配置 S3")
	}
	u, _ := url.Parse(config.Endpoint)
	lookup := minio.BucketLookupAuto
	if config.PathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""), Secure: u.Scheme == "https", Region: config.Region, BucketLookup: lookup})
	if err != nil {
		return nil, err
	}
	return &S3{client: client, config: config}, nil
}

// Check exercises write, read, list and delete permissions using a disposable object.
func (s *S3) Check(ctx context.Context) error {
	key := path.Join(s.config.Prefix, ".connection-test-"+domain.NewID())
	if _, err := s.client.PutObject(ctx, s.config.Bucket, key, strings.NewReader("resty-manager"), 13, minio.PutObjectOptions{}); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = s.client.RemoveObject(cleanup, s.config.Bucket, key, minio.RemoveObjectOptions{})
	}()
	obj, err := s.client.GetObject(ctx, s.config.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer obj.Close()
	b, err := io.ReadAll(io.LimitReader(obj, 64))
	if err != nil {
		return err
	}
	if string(b) != "resty-manager" {
		return fmt.Errorf("S3 回读校验失败")
	}
	for item := range s.client.ListObjects(ctx, s.config.Bucket, minio.ListObjectsOptions{Prefix: key, MaxKeys: 1}) {
		if item.Err != nil {
			return item.Err
		}
	}
	return s.client.RemoveObject(ctx, s.config.Bucket, key, minio.RemoveObjectOptions{})
}

func (s *S3) Upload(ctx context.Context, file string, record Record) (string, error) {
	key := path.Join(s.config.Prefix, record.ID+".tar.gz")
	_, err := s.client.FPutObject(ctx, s.config.Bucket, key, file, minio.PutObjectOptions{ContentType: "application/gzip", UserMetadata: map[string]string{"sha256": record.SHA256, "revision": fmt.Sprint(record.Revision)}})
	return key, err
}
func (s *S3) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	prefix := strings.Trim(s.config.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, ".tar.gz") {
		return nil, fmt.Errorf("无效 S3 对象键")
	}
	return s.client.GetObject(ctx, s.config.Bucket, key, minio.GetObjectOptions{})
}
func (s *S3) List(ctx context.Context) ([]Record, error) {
	items := []Record{}
	prefix := strings.Trim(s.config.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	for item := range s.client.ListObjects(ctx, s.config.Bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if item.Err != nil {
			return nil, item.Err
		}
		name := strings.TrimSuffix(path.Base(item.Key), ".tar.gz")
		if !domain.ValidID(name) || !strings.HasSuffix(item.Key, ".tar.gz") {
			continue
		}
		info, err := s.client.StatObject(ctx, s.config.Bucket, item.Key, minio.StatObjectOptions{})
		if err != nil {
			return nil, err
		}
		revision, _ := strconv.ParseUint(info.UserMetadata["Revision"], 10, 64)
		items = append(items, Record{ID: name, CreatedAt: item.LastModified, Size: item.Size, Revision: revision, Remote: true, ObjectKey: item.Key, SHA256: info.UserMetadata["Sha256"]})
		if len(items) > 10000 {
			return nil, fmt.Errorf("S3 备份数量超过 10000，请缩小前缀")
		}
	}
	return items, nil
}
