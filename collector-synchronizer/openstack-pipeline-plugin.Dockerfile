# bitflowstream/bitflow-pipeline
# Build from root of the repository:
# docker build -t bitflowstream/bitflow-pipeline -f build/multi-stage/alpine-full.Dockerfile .
FROM golang:1.14.1-alpine as build
RUN apk --no-cache add curl bash git mercurial gcc g++ docker musl-dev
WORKDIR /build
ENV GO111MODULE=on

RUN  go get github.com/bitflow-stream/go-bitflow/...

RUN ls /go/bin

# Copy rest of the source code and build
COPY . /go-bitflow/

RUN cd /go-bitflow && sed -i $(find -name go.mod) -e '\_//.*gitignore$_d' -e '\_#.*gitignore$_d'

RUN cd /go-bitflow && go mod download

FROM alpine:3.11.5
COPY --from=build /go/bin/bitflow-pipeline /
COPY --from=build /go-bitflow/collector-synchronizer/collector-synchronizer /
ENTRYPOINT ["/bin/sh", "-c", "/bitflow-pipeline -q $CommandParams"]
# Entry point sould has the basic command and then you can pass on the parameters for it to run the bitflow collector...