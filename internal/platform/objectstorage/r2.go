package objectstorage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const maxBytes = 5 * 1024 * 1024

var errUnavailable = errors.New("object storage unavailable")

type S3Client interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}
type R2 struct {
	client S3Client
	bucket string
}

func New(account, bucket, key, secret string, timeout time.Duration) *R2 {
	c := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String("https://" + account + ".r2.cloudflarestorage.com"), Credentials: credentials.NewStaticCredentialsProvider(key, secret, ""), UsePathStyle: true, HTTPClient: &http.Client{Timeout: timeout}, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired})
	return &R2{c, bucket}
}
func (r *R2) Put(ctx context.Context, key, mime string, data []byte) error {
	if len(data) == 0 || len(data) > maxBytes {
		return errUnavailable
	}
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key), ContentType: aws.String(mime), ContentLength: aws.Int64(int64(len(data))), CacheControl: aws.String("no-store"), Body: bytes.NewReader(data)})
	if err != nil {
		return errUnavailable
	}
	return nil
}
func (r *R2) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	if err != nil || out == nil || out.Body == nil {
		return nil, errUnavailable
	}
	defer out.Body.Close()
	data, err := io.ReadAll(io.LimitReader(out.Body, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		return nil, errUnavailable
	}
	return data, nil
}
func (r *R2) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	if err != nil {
		return errUnavailable
	}
	return nil
}
