ARG PACKAGE=github.com/brimstone/pocket
FROM brimstone/golang-musl as builder

FROM scratch
ARG BUILD_DATE
ARG VCS_REF
LABEL org.label-schema.build-date=$BUILD_DATE \
      org.label-schema.vcs-url="https://github.com/brimstone/docker-kali" \
      org.label-schema.vcs-ref=$VCS_REF \
      org.label-schema.schema-version="1.0.0-rc1"
ENTRYPOINT ["/pocket"]
ENV GITHUB.TOKEN= \
    POCKET.KEY= \
    POCKET.TOKEN= \
    MASTODON.CLIENT-ID= \
    MASTODON.CLIENT-SECRET= \
    MASTODON.USERNAME= \
    MASTODON.PASSWORD= \
    FREQUENCY=
COPY --from=builder /app /pocket
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
