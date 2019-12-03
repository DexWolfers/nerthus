
BASE=$GOPATH/src/gitee.com/nerthus/nerthus
remoteBase=/opt/gopath/src/gitee.com/nerthus/nerthus
images=nerthus/nerthus-baseimage:x86_64-0.0.1
docker run --rm -ti -v $BASE:$remoteBase $images  bash -c  "cd $remoteBase && go build gitee.com/nerthus/nerthus/app/cnts" 
