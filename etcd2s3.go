package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/revinate/etcd2s3/Godeps/_workspace/src/github.com/aws/aws-sdk-go/aws"
	"github.com/revinate/etcd2s3/Godeps/_workspace/src/github.com/aws/aws-sdk-go/aws/session"
	"github.com/revinate/etcd2s3/Godeps/_workspace/src/github.com/aws/aws-sdk-go/service/s3"
)

var bucketEnvVar = "ETCD2S3_BUCKET_NAME"
var etcd2DataDirEnvVar = "ETCD2S3_DATA_DIR"

func main() {

	// TODO: input validation
	// TODO: validate datadir is an actual directory
	// complain if required environment variables are absent
	req_envs := []string{"AWS_SECRET_ACCESS_KEY",
		"AWS_ACCESS_KEY_ID",
		bucketEnvVar,
		etcd2DataDirEnvVar}

	missing := make([]string, 0)
	for _, v := range req_envs {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}

	// tell the user and quit if we're missing anything
	if len(missing) > 0 {
		msg := "Missing " + strings.Join(missing, ", ") + " environment variable(s)."
		log.Fatal(msg)
	}

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
	cmd := exec.Command("/usr/bin/etcdctl", "backup", "--data-dir="+os.Getenv(etcd2DataDirEnvVar), backupdir)
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
	bucket := os.Getenv(bucketEnvVar)
	svc := s3.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))
	//svc := s3.New(session.New())

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
