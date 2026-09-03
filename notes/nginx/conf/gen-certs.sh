#!/bin/bash
# 生成 nginx 用的自签证书（原理见 notes/tls/05-证书体系.md）
# 用法：./gen-certs.sh（任意目录可跑，产物固定在脚本同级 certs/ 下）
# 产物：certs/server.crt + certs/server.key（已被 .gitignore 忽略，勿提交私钥）
set -e
cd "$(dirname "$0")"
mkdir -p certs
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
    -keyout certs/server.key -out certs/server.crt \
    -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
chmod 600 certs/server.key
echo "证书已生成:"
echo "  certs/server.crt  (证书，含 SAN: localhost / 127.0.0.1)"
echo "  certs/server.key  (私钥，权限 600)"
echo "验证: openssl x509 -in certs/server.crt -noout -subject -dates -ext subjectAltName"
