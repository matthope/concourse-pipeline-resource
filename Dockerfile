FROM golang:alpine as builder

ARG CONCOURSE_RELEASE_TAG="latest"

RUN apk add --no-cache curl jq
RUN mkdir -p /assets

RUN echo "https://api.github.com/repos/concourse/concourse/releases/${CONCOURSE_RELEASE_TAG}"

RUN url=$(curl -s "https://api.github.com/repos/concourse/concourse/releases/${CONCOURSE_RELEASE_TAG}" \
    | jq -r '.assets[] | select(.name | test("fly_linux_amd64")) | .browser_download_url') &&\
    curl -L "$url" -o /assets/fly
COPY . /go/src/github.com/concourse/concourse-pipeline-resource
ENV CGO_ENABLED 0
RUN go build -o /assets/in github.com/concourse/concourse-pipeline-resource/cmd/in
RUN go build -o /assets/out github.com/concourse/concourse-pipeline-resource/cmd/out
RUN go build -o /assets/check github.com/concourse/concourse-pipeline-resource/cmd/check
RUN set -e; for pkg in $(go list ./... | grep -v "acceptance"); do \
		go test -o "/tests/$(basename $pkg).test" -c $pkg; \
	done

FROM alpine:edge AS resource
RUN apk add --no-cache bash tzdata ca-certificates
COPY --from=builder assets/ /opt/resource/
RUN chmod +x /opt/resource/*

FROM resource AS tests
COPY --from=builder /tests /go-tests
RUN set -e; for test in /go-tests/*.test; do \
		$test; \
	done

FROM resource
