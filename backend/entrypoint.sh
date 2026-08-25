#!/bin/sh
set -eu

mkdir -p /data/attachments
chown -R workbench:workbench /data

exec su-exec workbench "$@"
