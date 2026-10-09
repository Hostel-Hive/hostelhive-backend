package objectstorage

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestR2SDKContract(t *testing.T) {
	methods := []string{}
	client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String("https://test.r2.cloudflarestorage.com"), UsePathStyle: true, Credentials: credentials.NewStaticCredentialsProvider("offline-key", "offline-secret", ""), RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired, HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.Method)
		if r.URL.Host != "test.r2.cloudflarestorage.com" || r.URL.Path != "/private-images/student-images/key" || !strings.Contains(r.Header.Get("Authorization"), "/auto/s3/aws4_request") {
			t.Fatal("wrong R2 signing or path")
		}
		if r.Method == "PUT" {
			data, _ := io.ReadAll(r.Body)
			if string(data) != "image" || r.Header.Get("Content-Type") != "image/png" || r.Header.Get("Cache-Control") != "no-store" || r.Header.Get("X-Amz-Acl") != "" {
				t.Fatal("unsafe upload metadata")
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("image"))}, nil
	})}})
	r := &R2{client, "private-images"}
	ctx := context.Background()
	if err := r.Put(ctx, "student-images/key", "image/png", []byte("image")); err != nil {
		t.Fatal(err)
	}
	data, err := r.Get(ctx, "student-images/key")
	if err != nil || string(data) != "image" {
		t.Fatal(err)
	}
	if err = r.Delete(ctx, "student-images/key"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "PUT,GET,DELETE" {
		t.Fatal(methods)
	}
}

func TestR2ErrorAndReadLimits(t *testing.T) {
	for _, mode := range []string{"provider-denied", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			c := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String("https://test.r2.cloudflarestorage.com"), UsePathStyle: true, Credentials: credentials.NewStaticCredentialsProvider("offline-key", "offline-secret", ""), RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired, HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				status := 200
				body := strings.Repeat("x", maxBytes+1)
				if mode == "provider-denied" {
					status = 403
					body = "<Error><Code>AccessDenied</Code><Message>private provider detail</Message></Error>"
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}})
			r := &R2{c, "private-images"}
			ctx := context.Background()
			if _, err := r.Get(ctx, "key"); err != errUnavailable {
				t.Fatal("unsafe retrieval", err)
			}
			if err := r.Put(ctx, "key", "image/png", nil); err != errUnavailable {
				t.Fatal("empty upload allowed")
			}
			if err := r.Put(ctx, "key", "image/png", make([]byte, maxBytes+1)); err != errUnavailable {
				t.Fatal("oversized upload allowed")
			}
			if mode == "provider-denied" {
				if err := r.Put(ctx, "key", "image/png", []byte("image")); err != errUnavailable {
					t.Fatal("provider error leaked", err)
				}
				if err := r.Delete(ctx, "key"); err != errUnavailable {
					t.Fatal("delete error leaked", err)
				}
			}
		})
	}
}

func TestR2Constructor(t *testing.T) {
	r := New(strings.Repeat("a", 32), "private-images", "offline-key", "offline-secret", time.Second)
	options := r.client.(*s3.Client).Options()
	if options.Region != "auto" || aws.ToString(options.BaseEndpoint) != "https://"+strings.Repeat("a", 32)+".r2.cloudflarestorage.com" || !options.UsePathStyle || options.HTTPClient.(*http.Client).Timeout != time.Second {
		t.Fatal("wrong R2 client configuration")
	}
}
