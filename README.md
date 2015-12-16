# etcd2s3

A simple tool to backup an etcd2 cluster into an AWS S3 bucket.

# Usage

##Get the code:
```
go get https://github.com/revinate/etc2s3
```

##Compile it:
```
cd $GOPATH/src/github.com/revinate/etcd2s3; go build
```

##Run it:
AWS S3 credentials are read from the following environment variables:

* AWS\_SECRET\_ACCESS\_KEY

* AWS\_ACCESS\_KEY\_ID

Once these are set with valid values:
```bash
./etcd2s3
```

# Roadmap
* use godeps
* make bucket an argument
* make etcd2 data dir an argument
* remove dependency on "tar"
* remove dependency on "etcdctl"
* remove dependence on disk for backup ("etcdctl backup" and "tar" save output to disk...can we stream directly?)
* use spf13/cobra or some other CLI building tool
* tests!
