###########################
# INSTRUCTIONS
############################
# BUILD
#	docker build -t nsip/otf-level:latest -t nsip/otf-level:v0.1.0 .
# TEST: docker run -it nsip/otf-level:latest .
# RUN: docker run -d nsip/otf-level:latest
#
# PUSH
#	Public:
#		docker push nsip/otf-level:v0.1.0
#		docker push nsip/otf-level:latest
#
#	Private:
#		docker tag nsip/otf-level:v0.1.0 the.hub.nsip.edu.au:3500/nsip/otf-level:v0.1.0
#		docker tag nsip/otf-level:latest the.hub.nsip.edu.au:3500/nsip/otf-level:latest
#		docker push the.hub.nsip.edu.au:3500/nsip/otf-level:v0.1.0
#		docker push the.hub.nsip.edu.au:3500/nsip/otf-level:latest
#
###########################
# DOCUMENTATION
############################

###########################
# STEP 0 Get them certificates
############################
# (note, step 2 is using alpine now) 
# FROM alpine:latest as certs

############################
# STEP 1 build executable binary (go.mod version)
############################
FROM golang:1.15.0-alpine3.12 as builder
RUN apk --no-cache add ca-certificates
RUN apk update && apk add git
RUN apk add gcc g++
RUN mkdir -p /build
WORKDIR /build
COPY . .
WORKDIR cmd/otf-level
RUN go build -o /build/app

############################
# STEP 2 build a small image
############################
#FROM debian:stretch
FROM alpine
COPY --from=builder /build/app /app
# NOTE - make sure it is the last build that still copies the files
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["./app"]
