FROM postgres:18-alpine
RUN apk add --no-cache bash aws-cli ca-certificates
COPY scripts/postgres-yedek.sh /backup.sh
USER 65532:65532
ENV BACKUP_DIR=/tmp/backups
ENTRYPOINT ["bash", "/backup.sh"]
