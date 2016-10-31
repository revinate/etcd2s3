def app = 'etcd2s3'
def registry = 'registry.revinate.net/techops'
def gopath = "/go/src/github.com/revinate/${app}"
def name = "${registry}/${app}"

stage 'Golang build'
node {
  checkout scm

  sh "docker run --rm -v `pwd`:${gopath} -w ${gopath} golang:1.7 make"

  stash name: 'binary', includes: "${app}"
}

stage 'Docker build and push'
node {
  checkout scm
  unstash 'binary'

  version = currentVersion()
  hoister.registry = registry
  hoister.imageName = app
  hoister.buildAndPush version

  stagehandPublish(app, version)
}
