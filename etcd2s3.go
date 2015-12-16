package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func main() {

	// create top level tempdir
	dirname := time.Now().Format(time.RFC3339)
	archiveName := dirname + ".tar.xz"

	fulldirpath, err := ioutil.TempDir(os.TempDir(), dirname)
	if err != nil {
		log.Fatal(err)
	}
	// cleanup after ourselves
	defer os.RemoveAll(fulldirpath)

	// create tempdir for backup
	databackupdir, err := ioutil.TempDir(fulldirpath, "data")
	if err != nil {
		log.Fatal(err)
	}

	// execute etcd2 backup
	backupdir := fmt.Sprint("--backup-dir=", databackupdir)
	cmd := exec.Command("/usr/bin/etcdctl", "backup", "--data-dir=/var/lib/etcd2", backupdir)
	log.Printf("Running: %s\n", cmd.Args)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("%s", output))
	}

	// compress
	pathToArchive := path.Join(fulldirpath, archiveName)
	cmd = exec.Command("/usr/bin/tar", "-Jcf", pathToArchive, databackupdir)
	log.Printf("Running: %s\n", cmd.Args)

	output, err = cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("%s", output))
	}

	// encrypt (TODO)

	// ship to s3
	bucket := "revinate.etcd2s3.backups"
	svc := s3.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))

	// check if bucket exists
	// TODO: bucket retention policy
	params := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}
	_, err = svc.HeadBucket(params)

	// if bucket doesn't exist, create it
	if err != nil {
		//actually create it
		_, err = svc.CreateBucket(&s3.CreateBucketInput{
			Bucket: &bucket,
		})
		if err != nil {
			log.Println("Failed to create bucket", err)
			return
		}

		if err = svc.WaitUntilBucketExists(&s3.HeadBucketInput{Bucket: &bucket}); err != nil {
			log.Printf("Failed to wait for bucket to exist %s, %s\n", bucket, err)
			return
		}
	}

	// get a file handle to the archive
	f, err := os.Open(pathToArchive)
	defer f.Close()

	// now, upload the file to the bucket
	_, err = svc.PutObject(&s3.PutObjectInput{
		Body:   f,
		Bucket: &bucket,
		Key:    &archiveName,
	})
	if err != nil {
		log.Printf("Failed to upload data to %s/%s, %s\n", bucket, archiveName, err)
		return
	}

}
