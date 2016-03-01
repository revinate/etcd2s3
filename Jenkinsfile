def app = 'etcd2s3'
def registry = 'registry.revinate.net/techops'
def gopath = "/go/src/github.com/revinate/${app}"
def name = "${registry}/${app}"

stage 'Golang build'
node {
  checkout scm

  sh "docker run --rm -v `pwd`:${gopath} -w ${gopath} golang:1.5 make"

  stash name: 'binary', includes: "${app}"
}

stage 'Docker build and push'
node {
  checkout scm

  unstash 'binary'

  sh "docker build -t ${name} ."

  sh "git log -1 | head -1 | cut -c 8-10 > git-revision"
  def revision = readFile 'git-revision'
  def version = "1.0.${env.BUILD_NUMBER ?: 0}.${revision ?: 1}"

  def tag = "${name}:${version}"
  sh "docker tag -f ${name} ${tag}"
  sh "docker push ${tag}"
  sh "docker push ${name}"

  sh "curl -s -XPOST http://stagehand-techops-prod.revinate.net/build/${app} -d 'version=${version}'"
}
