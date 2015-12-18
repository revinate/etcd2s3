package main

import (
	"archive/tar"
	"compress/gzip"
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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

var bucketEnvVar = "ETCD2S3_BUCKET_NAME"
var etcd2DataDirEnvVar = "ETCD2S3_DATA_DIR"

func main() {

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
		log.Fatal(msg)
	}

	// create top level tempdir
	dirname := time.Now().Format(time.RFC3339)
	archiveName := dirname + ".tar.bz"

	fulldirpath, err := ioutil.TempDir(os.TempDir(), dirname)
	if err != nil {
		log.Fatal(err)
	}
	// cleanup after ourselves
	defer os.RemoveAll(fulldirpath)

	// create tempdir for backup
	databackupdir := path.Join(fulldirpath, "data")
	err = os.Mkdir(databackupdir, 0700) //ioutil.TempDir(fulldirpath, "data")
	if err != nil {
		log.Fatal(err)
	}

	// execute etcd2 backup
	backupdir := fmt.Sprint("--backup-dir=", databackupdir)
	cmd := exec.Command("/usr/bin/etcdctl", "backup", "--data-dir="+os.Getenv(etcd2DataDirEnvVar), backupdir)
	log.Printf("Taking backup: %s\n", strings.Join(cmd.Args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("%s", output))
	}

	// compress
	pathToArchive := path.Join(fulldirpath, archiveName)
	log.Print("Compressing...")
	wrapit(databackupdir, pathToArchive)

	// encrypt (TODO)

	// ship to s3
	bucket := os.Getenv(bucketEnvVar)
	svc := s3.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))

	// check if bucket exists
	// TODO: bucket retention policy
	params := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}
	log.Printf("Verifying bucket \"%s\" exists.\n", bucket)
	_, err = svc.HeadBucket(params)

	// if bucket doesn't exist, create it
	if err != nil {
		log.Print("Bucket missing.  Creating...")
		//actually create it
		_, err = svc.CreateBucket(&s3.CreateBucketInput{
			Bucket: &bucket,
		})
		if err != nil {
			log.Fatal("Failed to create bucket: ", err)
		}

		if err = svc.WaitUntilBucketExists(&s3.HeadBucketInput{Bucket: &bucket}); err != nil {
			log.Fatal("Failed to wait for bucket to exist: ", bucket, err)
		}
	}

	// get a file handle to the archive
	f, err := os.Open(pathToArchive)
	defer f.Close()

	log.Print("Sending backup to S3...")
	// now, upload the file to the bucket
	_, err = svc.PutObject(&s3.PutObjectInput{
		Body:   f,
		Bucket: &bucket,
		Key:    &archiveName,
	})
	if err != nil {
		log.Fatal("Failed to upload data to S3: ", bucket, archiveName, err)
	}

	log.Print("Success.")

	//	wrapit("/tmp/foo", "/tmp/foo.tar.bz")

}

func wrapit(source, target string) error {
	pr, pw := io.Pipe()

	// create channels to synchronize
	done := make(chan bool)
	errs := make(chan error)
	defer close(done)
	defer close(errs)

	// combine to single stream
	go tarit(source, pw, errs, done)

	// compress stream and save to file
	// TODO: use xz when golang has native
	// xz code...currently, only libc bindings
	go gzipit(pr, target, errs, done)

	// wait until both are done
	// or an error occurs
	for c := 0; c < 2; {
		select {
		case err := <-errs:
			return err
		case <-done:
			c++
		}
	}

	return nil
}

// func tarit() is based on this article:
// http://blog.ralch.com/tutorial/golang-working-with-tar-and-gzip/
func tarit(source string, target io.WriteCloser, errs chan error, done chan bool) error {
	// must close the target when done
	defer target.Close()

	tarball := tar.NewWriter(target)
	defer tarball.Close()

	info, err := os.Stat(source)
	if err != nil {
		log.Fatal(err) //return nil
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	// dive into each file/directory
	filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				errs <- err //log.Fatal(err) //return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				errs <- err //log.Fatal(err) //return err
			}

			if baseDir != "" {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			}

			if err := tarball.WriteHeader(header); err != nil {
				errs <- err //log.Fatal(err) //return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				errs <- err //log.Fatal(err) //return err
			}
			defer file.Close()
			_, err = io.Copy(tarball, file)
			if err != nil {
				errs <- err
			}

			return err
		})

	done <- true

	return nil
}

// func gzipit() is based on this article:
// http://blog.ralch.com/tutorial/golang-working-with-tar-and-gzip/
func gzipit(reader io.Reader, target string, errs chan error, done chan bool) error {
	writer, err := os.Create(target)
	if err != nil {
		errs <- err //log.Fatal(err) //return err
	}
	defer writer.Close()

	archiver := gzip.NewWriter(writer)
	defer archiver.Close()

	_, err = io.Copy(archiver, reader)
	if err != nil {
		errs <- err
	}

	done <- true

	return nil
}
