BINS=etcd2s3

$(BINS): etcd2s3.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo .

clean:
	rm -rf $(BINS)

all: clean $(BINS)
