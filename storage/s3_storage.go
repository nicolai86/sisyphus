package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Storage struct {
	Bucket string
}

func NewS3Storage(bucket string) S3Storage {
	return S3Storage{
		Bucket: bucket,
	}
}

func (f S3Storage) Store(r Repository) error {
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: os.Getenv("AWS_PROFILE"),
	})
	if err != nil {
		return err
	}

	svc := s3.New(sess)

	if len(r.Plugins) == 0 {
		params := &s3.DeleteObjectInput{
			Bucket: aws.String(f.Bucket),
			Key:    aws.String(fmt.Sprintf("%s.json", r.ID)),
		}
		_, err := svc.DeleteObject(params)
		return err
	}

	bs, err := json.MarshalIndent(&r, "", "\t")
	if err != nil {
		return err
	}
	params := &s3.PutObjectInput{
		Bucket: aws.String(f.Bucket),
		Key:    aws.String(fmt.Sprintf("%s.json", r.ID)),
		Body:   bytes.NewReader(bs),
	}
	_, err = svc.PutObject(params)

	return err
}

func (f S3Storage) Load() ([]Repository, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: os.Getenv("AWS_PROFILE"),
	})
	if err != nil {
		return nil, fmt.Errorf("Unable to get S3 session: %q", err)
	}

	svc := s3.New(sess)

	params := &s3.ListObjectsInput{
		Bucket: aws.String(f.Bucket),
	}
	resp, err := svc.ListObjects(params)
	if err != nil {
		return nil, fmt.Errorf("Unable to list objects: %q", err)
	}

	var repos []Repository
	for _, o := range resp.Contents {
		params := &s3.GetObjectInput{
			Bucket: aws.String(f.Bucket),
			Key:    aws.String(*o.Key),
		}
		resp, err := svc.GetObject(params)

		if err != nil {
			return nil, err
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var r Repository
		json.Unmarshal(b, &r)
		repos = append(repos, r)
	}

	return repos, nil
}
