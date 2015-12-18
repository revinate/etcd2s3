package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/revinate/etcd2s3/Godeps/_workspace/src/github.com/aws/aws-sdk-go/aws"
	"github.com/revinate/etcd2s3/Godeps/_workspace/src/github.com/aws/aws-sdk-go/aws/session"
	"github.com/revinate/etcd2s3/Godeps/_workspace/src/github.com/aws/aws-sdk-go/service/s3"
	"github.com/revinate/etcd2s3/Godeps/_workspace/src/github.com/aws/aws-sdk-go/service/s3/s3manager"
)

var bucketEnvVar = "ETCD2S3_BUCKET_NAME"
var etcd2DataDirEnvVar = "ETCD2S3_DATA_DIR"
var repeatEnvVar = "ETCD2S3_REPEAT_INTERVAL"

func main() {
	// help users
	err := validate()
	if err != nil {
		log.Fatal(err)
	}

	// first run
	// ignore failures
	err = backupAndShip()
	if err != nil {
		log.Print(err)
	}

	// repeat, if necessary
	repeat()
}

func validate() error {
	// TODO: input validation
	// TODO: validate datadir is an actual directory
	// complain if required environment variables are absent
	reqEnvs := []string{"AWS_SECRET_ACCESS_KEY",
		"AWS_ACCESS_KEY_ID",
		bucketEnvVar,
		etcd2DataDirEnvVar}

	var missing []string
	for _, v := range reqEnvs {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}

	// tell the user and quit if we're missing anything
	if len(missing) > 0 {
		msg := "Missing " + strings.Join(missing, ", ") + " environment variable(s)."
		return errors.New(msg)
	}

	return nil
}

func repeat() error {
	repeatInterval := os.Getenv(repeatEnvVar)

	if repeatInterval != "" {
		_repeatDuration, err := time.ParseDuration(repeatInterval)
		if err != nil {
			return err
		}

		// set ticker duration
		ticker := time.NewTicker(_repeatDuration)

		// repeated runs, based on duration of ticker
		for {
			log.Printf("Next execution will be at %s (%s from now).", time.Now().Add(_repeatDuration).Format(time.RFC3339), _repeatDuration)
			select {
			case <-ticker.C:
				log.Print("Waking up to backup.")
				go backupAndShip()
			}
		}
	} else {
		log.Print("EnvVar " + repeatEnvVar + " is missing.  Not repeating.")
	}

	return nil
}

func backupAndShip() error {
	// create top level tempdir
	dirname := time.Now().Format(time.RFC3339)
	archiveName := dirname + ".tar.bz"

	fulldirpath, err := ioutil.TempDir(os.TempDir(), dirname)
	if err != nil {
		return err
	}
	// cleanup after ourselves
	defer os.RemoveAll(fulldirpath)

	// create tempdir for backup
	databackupdir := path.Join(fulldirpath, "data")
	err = os.Mkdir(databackupdir, 0700)
	if err != nil {
		return err
	}

	// execute etcd2 backup
	backupdir := fmt.Sprint("--backup-dir=", databackupdir)
	cmd := exec.Command("/usr/bin/etcdctl", "backup", "--data-dir="+os.Getenv(etcd2DataDirEnvVar), backupdir)
	log.Printf("Taking backup: %s\n", strings.Join(cmd.Args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(string(output) + err.Error())
	}

	// pipes to make this quick!
	combineReader, combineWriter := io.Pipe()
	compressReader, compressWriter := io.Pipe()

	// create channels to synchronize
	done := make(chan bool)
	errs := make(chan error)
	defer close(done)
	defer close(errs)

	go tarIt(databackupdir, combineWriter, errs, done)
	go compressIt(combineReader, compressWriter, errs, done)
	//TODO: encryptit()
	go shipIt(compressReader, os.Getenv(bucketEnvVar), archiveName, errs, done)

	// TODO: refactor this to be more elegant
	// wait for all to finish
	for c := 0; c < 3; {
		select {
		case err := <-errs:
			return err
		case <-done:
			c++
		}
	}

	log.Print("Success.")

	return nil

}

func shipIt(reader io.Reader, bucket string, key string, errs chan error, done chan bool) {
	err := ensureBucketExists(bucket)
	if err != nil {
		errs <- err
	}

	uploader := s3manager.NewUploader(session.New(&aws.Config{Region: aws.String("us-west-2")}))
	_, err = uploader.Upload(&s3manager.UploadInput{
		Body:   reader,
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		errs <- err
	}

	done <- true

}

// func tarIt() is based on this article:
// http://blog.ralch.com/tutorial/golang-working-with-tar-and-gzip/
func tarIt(source string, target io.WriteCloser, errs chan error, done chan bool) {
	// must close the target when done
	defer target.Close()

	tarball := tar.NewWriter(target)
	defer tarball.Close()

	info, err := os.Stat(source)
	if err != nil {
		log.Fatal(err)
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	// dive into each file/directory
	filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				errs <- err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				errs <- err
			}

			if baseDir != "" {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			}

			if err := tarball.WriteHeader(header); err != nil {
				errs <- err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				errs <- err
			}
			defer file.Close()
			_, err = io.Copy(tarball, file)
			if err != nil {
				errs <- err
			}

			return err
		})

	done <- true

}

// func gzipit() is based on this article:
// http://blog.ralch.com/tutorial/golang-working-with-tar-and-gzip/
func compressIt(reader io.Reader, writer io.WriteCloser, errs chan error, done chan bool) {
	defer writer.Close()

	archiver := gzip.NewWriter(writer)
	defer archiver.Close()

	_, err := io.Copy(archiver, reader)
	if err != nil {
		errs <- err
	}

	done <- true

}

func ensureBucketExists(bucket string) error { //bucket string) {
	svc := s3.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))

	// check if bucket exists
	// TODO: bucket retention policy
	params := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}
	log.Printf("Verifying bucket \"%s\" exists.\n", bucket)
	_, err := svc.HeadBucket(params)

	// if bucket doesn't exist, create it
	if err != nil {
		log.Print("Bucket missing.  Creating...")
		//actually create it
		_, err = svc.CreateBucket(&s3.CreateBucketInput{
			Bucket: &bucket,
		})
		if err != nil {
			return err
		}

		if err = svc.WaitUntilBucketExists(&s3.HeadBucketInput{Bucket: &bucket}); err != nil {
			return err
		}
	}

	return nil

}
