FROM alpine:latest

# install ca-certificates
RUN apk --update --no-cache add ca-certificates

# add binary
ADD build/linux/buddy /

ENTRYPOINT ["/buddy"]
