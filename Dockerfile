FROM golang:1.27-alpine AS ssm
WORKDIR /src
COPY ssmget/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /ssmget .

FROM restic/restic:0.19.1
RUN apk add --no-cache curl postgresql18-client
COPY --from=ssm /ssmget /usr/local/bin/ssmget
COPY scripts/archivist-key /usr/local/bin/archivist-key
COPY scripts/archivist-entrypoint.sh /archivist-entrypoint.sh
ENTRYPOINT ["/archivist-entrypoint.sh"]
