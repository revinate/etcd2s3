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
Required environment variables:

* AWS\_SECRET\_ACCESS\_KEY

* AWS\_ACCESS\_KEY\_ID

* ETCD2S3\_BUCKET\_NAME

* ETCD2S3\_DATA\_DIR - the path to the etcd2 data directory (where the backup utility will start from)

Optional environment variables:

* ETCD2S3\_REPEAT\_INTERVAL - if set using [time.ParseDuration](https://golang.org/pkg/time/#ParseDuration), this will keep the utility running and attempt a backup after each interval.  For example, if set to "5m", the utility will run every 5 minutes.

Once these are set with valid values:
```bash
./etcd2s3
```

# Roadmap
* remove dependency on "etcdctl"
* remove dependence on disk for backup ("etcdctl backup"...can we stream directly to tar/zipper?)
* use spf13/cobra or some other CLI building tool
* tests!
