FROM alpine:3.7

# GOLANG install steps used from the official golang:1.9-alpine image
# at https://github.com/docker-library/golang/blob/master/1.9/alpine3.7/Dockerfile

RUN apk add --no-cache ca-certificates

ENV GOLANG_VERSION 1.9.4

# no-pic.patch: https://golang.org/issue/14851 (Go 1.8 & 1.7)
COPY *.patch /go-alpine-patches/

RUN set -eux; \
    apk add --no-cache --virtual .build-deps \
        bash \
        gcc \
        musl-dev \
        openssl \
        go \
    ; \
    export \
# set GOROOT_BOOTSTRAP such that we can actually build Go
        GOROOT_BOOTSTRAP="$(go env GOROOT)" \
# ... and set "cross-building" related vars to the installed system's values so that we create a build targeting the proper arch
# (for example, if our build host is GOARCH=amd64, but our build env/image is GOARCH=386, our build needs GOARCH=386)
        GOOS="$(go env GOOS)" \
        GOARCH="$(go env GOARCH)" \
        GOHOSTOS="$(go env GOHOSTOS)" \
        GOHOSTARCH="$(go env GOHOSTARCH)" \
    ; \
# also explicitly set GO386 and GOARM if appropriate
# https://github.com/docker-library/golang/issues/184
    apkArch="$(apk --print-arch)"; \
    case "$apkArch" in \
        armhf) export GOARM='6' ;; \
        x86) export GO386='387' ;; \
    esac; \
    \
    wget -O go.tgz "https://golang.org/dl/go$GOLANG_VERSION.src.tar.gz"; \
    echo '0573a8df33168977185aa44173305e5a0450f55213600e94541604b75d46dc06 *go.tgz' | sha256sum -c -; \
    tar -C /usr/local -xzf go.tgz; \
    rm go.tgz; \
    \
    cd /usr/local/go/src; \
    for p in /go-alpine-patches/*.patch; do \
        [ -f "$p" ] || continue; \
        patch -p2 -i "$p"; \
    done; \
    ./make.bash; \
    \
    rm -rf /go-alpine-patches; \
    apk del .build-deps; \
    \
    export PATH="/usr/local/go/bin:$PATH"; \
    go version

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"
WORKDIR $GOPATH

# Controller install steps
COPY entrypoint.builder.sh /entrypoint.sh

RUN apk add --no-cache \
        bash \
        git \
        make \
        rsync \
        su-exec && \
    go get github.com/wadey/gocovmerge && \
    go get golang.org/x/tools/cmd/cover && \
    go get github.com/mattn/goveralls && \
    go get github.com/onsi/ginkgo/ginkgo && \
    go get github.com/onsi/gomega && \
    chmod 755 /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]
CMD [ "/bin/bash" ]
